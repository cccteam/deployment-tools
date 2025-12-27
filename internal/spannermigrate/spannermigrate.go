package spannermigrate

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/spanner"
	spannerDB "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/cccteam/spxscan"
	"github.com/go-playground/errors/v5"
	"github.com/zredinger-ccc/migrate/v4"
	"github.com/zredinger-ccc/migrate/v4/database"
	spannerDriver "github.com/zredinger-ccc/migrate/v4/database/spanner"
	_ "github.com/zredinger-ccc/migrate/v4/source/file" // up/down script file source driver for the migrate package
	"google.golang.org/api/option"
)

const defaultSchemaVersion = -1

type Client struct {
	dbStr          string
	admin          *spannerDB.DatabaseAdminClient
	client         *spanner.Client
	migrateClients []*migrate.Migrate // migrateClients is used to track migrate clients and cleanup their resources
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
func (s *Client) MigrateUpSchema(ctx context.Context, sourceURL string) error {
	m, err := s.newMigrate(sourceURL)
	if err != nil {
		return errors.Wrap(err, "migrateUp()")
	}

	if err := m.Up(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Up(): %s", sourceURL)
	}

	return nil
}

// MigrateUpData will apply all migrations without changing the migration version.
// This should be used for data migrations. (DML)
func (s *Client) MigrateUpData(ctx context.Context, sourceURLs ...string) error {
	var schemaMigration struct {
		Version int64 `spanner:"Version"`
		Dirty   bool  `spanner:"Dirty"`
	}
	_, err := s.client.ReadWriteTransaction(ctx,
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
		return &database.Error{OrigErr: err}
	}

	if schemaMigration.Dirty {
		return errors.New("schema migration is dirty")
	}

	log.Printf("Reset migrations from %d to %d", schemaMigration.Version, defaultSchemaVersion)

	for _, sourceURL := range sourceURLs {
		if err := s.migrateUp(sourceURL); err != nil {
			return errors.Wrapf(err, "MigrateUpBlind: %s", sourceURL)
		}
	}

	_, err = s.client.ReadWriteTransaction(ctx,
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
		log.Printf("ERROR: failed to reset schema migration version, please check the database")

		return errors.Wrap(err, "failed to reset schema migration version")
	}

	log.Printf("Reset migrations from %d to %d", defaultSchemaVersion, schemaMigration.Version)

	return nil
}

func (s *Client) migrateUp(sourceURL string) error {
	m, err := s.newMigrate(sourceURL)
	if err != nil {
		return errors.Wrap(err, "migrateUp()")
	}

	if err := m.Up(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Up(): %s", sourceURL)
	}

	return nil
}

func (s *Client) MigrateDropSchema(ctx context.Context, sourceURL string) error {
	conf := &spannerDriver.Config{DatabaseName: s.dbStr, CleanStatements: true}
	spannerInstance, err := spannerDriver.WithInstance(spannerDriver.NewDB(*s.admin, *s.client), conf)
	if err != nil {
		return errors.Wrap(err, "spannerDriver.WithInstance()")
	}

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "spanner", spannerInstance)
	if err != nil {
		return errors.Wrapf(err, "migrate.NewWithDatabaseInstance(): fileURL=%s, db=%s", sourceURL, s.dbStr)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("migrate.Migrate.Close() error: source error: %v, database error: %v: %s", srcErr, dbErr, sourceURL)
		}
		if dbErr != nil {
			log.Printf("migrate.Migrate.Close() error: database error: %v: %s", dbErr, sourceURL)
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

func (s *Client) Close() {
	for _, m := range s.migrateClients {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Println("failed to close source", srcErr)
		}
		if dbErr != nil {
			log.Println("failed to close database", dbErr)
		}
	}
	if err := s.admin.Close(); err != nil {
		log.Println("failed to close admin client", err)
	}
	s.client.Close()
}

// newMigrate creates a new migrate instance and registers it with the migrateClients on Client
func (s *Client) newMigrate(sourceURL string) (*migrate.Migrate, error) {
	conf := &spannerDriver.Config{DatabaseName: s.dbStr, CleanStatements: true}
	spannerInstance, err := spannerDriver.WithInstance(spannerDriver.NewDB(*s.admin, *s.client), conf)
	if err != nil {
		return nil, errors.Wrap(err, "spannerDriver.WithInstance()")
	}

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "spanner", spannerInstance)
	if err != nil {
		return nil, errors.Wrapf(err, "migrate.NewWithDatabaseInstance(): fileURL=%s, db=%s", sourceURL, s.dbStr)
	}

	s.migrateClients = append(s.migrateClients, m)

	return m, nil
}
