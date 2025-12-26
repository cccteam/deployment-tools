package bootstrap

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/spanner"
	spannerDB "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/cccteam/spxscan"
	"github.com/go-playground/errors/v5"
	"github.com/golang-migrate/migrate/v4"
	spannerDriver "github.com/golang-migrate/migrate/v4/database/spanner"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
)

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := command{}
	return cli.Setup(ctx)
}

type command struct {
	dataMigrationDirs  []string
	SchemaMigrationDir string
}

// Setup returns the configured cli command
func (c *command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap database",
		Long:  "Bootstrap database by running specified migrations",
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			if err := c.ValidateFlags(cmd); err != nil {
				return err
			}

			if err := c.Run(ctx, cmd); err != nil {
				log.Println(err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&c.SchemaMigrationDir, "schema-dir", "s", "file://schema/migrations", "Directory containing schema migration files, using the file URI syntax")
	cmd.Flags().StringSliceVar(&c.dataMigrationDirs, "data-dirs", []string{"file://bootstrap/testdata"}, "Directories containing data migration files, using the file URI syntax")

	return cmd
}

// ValidateFlags validates and processes any input flags
func (c *command) ValidateFlags(cmd *cobra.Command) error {
	// Validate schema migration directory
	if c.SchemaMigrationDir == "" {
		return errors.New("schema-dir flag is required")
	}

	// Validate data migration directories
	if len(c.dataMigrationDirs) == 0 {
		return errors.New("at least one data-dir flag is required")
	}

	return nil
}

// Run executes the command
func (c *command) Run(ctx context.Context, cmd *cobra.Command) error {
	conf, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer conf.close()

	if err := conf.migrateClient.MigrateUpSchema(ctx, c.SchemaMigrationDir); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to run schema migrations")
	}

	if err := conf.migrateClient.MigrateUpData(ctx, c.dataMigrationDirs...); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to failed to run migrations")
	} else if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("No new Migration scripts found. No changes applied.")
	} else {
		fmt.Println("Ran migration successful")
	}

	return nil
}

type SpannerMigrationService struct {
	dbStr  string
	admin  *spannerDB.DatabaseAdminClient
	client *spanner.Client
}

// ConnectToSpanner connects to an existing spanner database and returns a SpannerMigrationService
func ConnectToSpanner(ctx context.Context, projectID, instanceID, dbName string, opts ...option.ClientOption) (*SpannerMigrationService, error) {
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

	return &SpannerMigrationService{
		dbStr:  dbStr,
		admin:  adminClient,
		client: client,
	}, nil
}

// MigrateUpSchema will migrate all the way up, applying all up migrations from the sourceURL.
// This should be used for schema migrations. (DDL)
func (s *SpannerMigrationService) MigrateUpSchema(ctx context.Context, sourceURL string) error {
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
		if srcErr, dbErr := m.Close(); err != nil {
			log.Printf("migrate.Migrate.Close() error: source error: %v, database error: %v: %s", srcErr, dbErr, sourceURL)
		}
	}()

	if err := m.Up(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Up(): %s", sourceURL)
	}

	if err, dbErr := m.Close(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Close(): source error: %s", sourceURL)
	} else if dbErr != nil {
		return errors.Wrapf(dbErr, "migrate.Migrate.Close(): database error: %s", sourceURL)
	}

	return nil
}

// MigrateUpData will apply all migrations while resetting the migrate version to the original state.
// This should be used for data migrations. (DML)
func (s *SpannerMigrationService) MigrateUpData(ctx context.Context, sourceURLs ...string) error {
	// first get the current version
	var curVersion int
	if err := spxscan.Get(ctx, s.client.Single(), &curVersion, spanner.NewStatement("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1")); err != nil {
		return errors.Wrap(err, "failed to get current schema version")
	}

	for _, sourceURL := range sourceURLs {
		if err := s.migrateUp(sourceURL); err != nil {
			return errors.Wrapf(err, "MigrateUpBlind: %s", sourceURL)
		}
	}

	if _, err := s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		stmt := spanner.NewStatement("UPDATE schema_migrations SET version = @version WHERE true")
		stmt.Params["version"] = curVersion
		if _, err := txn.Update(ctx, stmt); err != nil {
			return errors.Wrapf(err, "failed to update schema_migrations version to %d", curVersion)
		}

		return nil
	}); err != nil {
		return errors.Wrapf(err, "failed to reset version to %d", curVersion)
	}

	return nil
}

func (s *SpannerMigrationService) migrateUp(sourceURL string) error {
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
		if srcErr, dbErr := m.Close(); err != nil {
			log.Printf("migrate.Migrate.Close() error: source error: %v, database error: %v: %s", srcErr, dbErr, sourceURL)
		}
	}()

	if err := m.Up(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Up(): %s", sourceURL)
	}

	if err, dbErr := m.Close(); err != nil {
		return errors.Wrapf(err, "migrate.Migrate.Close(): source error: %s", sourceURL)
	} else if dbErr != nil {
		return errors.Wrapf(dbErr, "migrate.Migrate.Close(): database error: %s", sourceURL)
	}

	return nil
}

func (s *SpannerMigrationService) Close() {
	if err := s.admin.Close(); err != nil {
		log.Printf("failed to close admin client: %v", err)
	}
	s.client.Close()
}
