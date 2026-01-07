package spannermigrate

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	spannerDB "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/cccteam/logger"
	"github.com/cccteam/spxscan"
	"github.com/go-playground/errors/v5"
	"github.com/zredinger-ccc/migrate/v4"
	spannerDriver "github.com/zredinger-ccc/migrate/v4/database/spanner"
	_ "github.com/zredinger-ccc/migrate/v4/source/file" // up/down script file source driver for the migrate package
	"google.golang.org/api/option"
)

const defaultSchemaVersion = -1

type Client struct {
	dbStr  string
	admin  *spannerDB.DatabaseAdminClient
	client *spanner.Client
}

// Connect connects to an existing spanner database and returns a Client
func Connect(ctx context.Context, projectID, instanceID, dbName string, opts ...option.ClientOption) (*Client, error) {
	dbStr := fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectID, instanceID, dbName)
	client, err := spanner.NewClient(ctx, dbStr, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "spanner.NewClient()")
	}

	adminClient, err := spannerDB.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		client.Close()

		return nil, errors.Wrap(err, "database.NewDatabaseAdminClient()")
	}

	return &Client{
		dbStr:  dbStr,
		admin:  adminClient,
		client: client,
	}, nil
}

// MigrateUpSchema will migrate all the way up, applying all up migrations from the sourceURL.
// This should be used for schema migrations. (DDL)
func (c *Client) MigrateUpSchema(ctx context.Context, sourceURL string) error {
	logger.FromCtx(ctx).Infof("Applying schema migrations from %s", sourceURL)
	m, err := c.newMigrate(sourceURL)
	if err != nil {
		return errors.Wrap(err, "Client.newMigrate()")
	}

	if err := m.Up(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Up(): %s", sourceURL)
	}

	return nil
}

// MigrateUpData will apply all migrations without changing the schema migration version in db.
// This should be used for data migrations. (DML)
func (c *Client) MigrateUpData(ctx context.Context, sourceURLs ...string) error {
	var schemaMigration struct {
		Version int64 `spanner:"Version"`
		Dirty   bool  `spanner:"Dirty"`
	}
	_, err := c.client.ReadWriteTransaction(ctx,
		func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			err := spxscan.Get(ctx, txn, &schemaMigration, spanner.NewStatement("SELECT Version, Dirty FROM SchemaMigrations"))
			if err != nil {
				return errors.Wrap(err, "spxscan.Get()")
			}

			m := []*spanner.Mutation{
				spanner.Delete("SchemaMigrations", spanner.AllKeys()),
				spanner.Insert("SchemaMigrations",
					[]string{"Version", "Dirty"},
					[]any{defaultSchemaVersion, false},
				),
			}

			return txn.BufferWrite(m)
		})
	if err != nil {
		return errors.Wrap(err, "failed to set schema migration version to default")
	}

	if schemaMigration.Dirty {
		return errors.New("schema migration is dirty. Fix this before continuing")
	}

	logger.FromCtx(ctx).Infof("Reset migrations from %d to %d", schemaMigration.Version, defaultSchemaVersion)

	for _, sourceURL := range sourceURLs {
		logger.FromCtx(ctx).Infof("Applying data migrations from %s", sourceURL)
		if err := c.migrateUp(sourceURL); err != nil {
			logger.FromCtx(ctx).Errorf("failed to apply data migrations from %s, migrations skipped...", sourceURL)
		}
	}

	_, err = c.client.ReadWriteTransaction(ctx,
		func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			m := []*spanner.Mutation{
				spanner.Delete("SchemaMigrations", spanner.AllKeys()),
				spanner.Insert("SchemaMigrations",
					[]string{"Version", "Dirty"},
					[]any{schemaMigration.Version, schemaMigration.Dirty},
				),
			}

			return txn.BufferWrite(m)
		})
	if err != nil {
		logger.FromCtx(ctx).Errorf("failed to restore schema migration version, please check db and reset to version %d", schemaMigration.Version)

		return errors.Wrap(err, "failed to restore schema migration version")
	}

	logger.FromCtx(ctx).Infof("Reset migrations from %d to %d", defaultSchemaVersion, schemaMigration.Version)

	return nil
}

func (c *Client) MigrateDropSchema(ctx context.Context, sourceURL string) error {
	m, err := c.newMigrate(sourceURL)
	if err != nil {
		return errors.Wrap(err, "Client.migrateUp()")
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			logger.FromCtx(ctx).Errorf("migrate.Migrate.Close() error: source error: %v: %s", srcErr, sourceURL)
		}
		if dbErr != nil {
			logger.FromCtx(ctx).Errorf("migrate.Migrate.Close() error: database error: %v: %s", dbErr, sourceURL)
		}
	}()

	if err := m.Drop(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Drop(): %s", sourceURL)
	}

	if err, dbErr := m.Close(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Close(): source error: %s", sourceURL)
	} else if dbErr != nil {
		return errors.Wrapf(dbErr, "migrate.Migrate.Close(): database error: %s", sourceURL)
	}

	return nil
}

func (c *Client) Close(ctx context.Context) {
	if err := c.admin.Close(); err != nil {
		logger.FromCtx(ctx).Errorf("failed to close admin client: %v", err)
	}
	c.client.Close()
}

func (c *Client) migrateUp(sourceURL string) error {
	m, err := c.newMigrate(sourceURL)
	if err != nil {
		return errors.Wrap(err, "Client.newMigrate()")
	}

	if err := m.Up(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Up(): %s", sourceURL)
	}

	return nil
}

// newMigrate creates a new migrate instance
func (c *Client) newMigrate(sourceURL string) (*migrate.Migrate, error) {
	conf := &spannerDriver.Config{DatabaseName: c.dbStr, CleanStatements: true}
	spannerInstance, err := spannerDriver.WithInstance(
		spannerDriver.NewDB(*c.admin, *c.client),
		conf,
	)
	if err != nil {
		return nil, errors.Wrap(err, "spannerDriver.WithInstance()")
	}

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "spanner", spannerInstance)
	if err != nil {
		return nil, errors.Wrapf(err, "migrate.NewWithDatabaseInstance(): fileURL=%s, db=%s", sourceURL, c.dbStr)
	}

	return m, nil
}
