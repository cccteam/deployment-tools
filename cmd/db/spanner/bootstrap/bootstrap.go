package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/errors/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/spf13/cobra"
)

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := command{}

	return cli.Setup(ctx)
}

type command struct {
	dataMigrationDirs   []string
	SchemaMigrationDirs []string
}

// Setup returns the configured cli command
func (c *command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap database, schema and data migrations",
		Long:  "Bootstrap database by running specified migrations. This will first run the schema migrations (if they are provided), followed by data migrations",
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			if err := c.ValidateFlags(cmd); err != nil {
				return err
			}

			if err := c.Run(ctx, cmd); err != nil {
				return errors.Wrap(err, "command.Run()")
			}

			return nil
		},
	}

	cmd.Flags().
		StringSliceVar(&c.SchemaMigrationDirs, "schema-dir", []string{"file://schema/migrations"}, "Directories containing schema migration files, using the file URI syntax")
	cmd.Flags().
		StringSliceVar(&c.dataMigrationDirs, "data-dir", []string{"file://bootstrap/testdata"}, "Directories containing data migration files, using the file URI syntax")

	return cmd
}

func (c *command) ValidateFlags(cmd *cobra.Command) error {
	return nil
}

func (c *command) Run(ctx context.Context, cmd *cobra.Command) error {
	conf, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer conf.close()

	switch len(c.SchemaMigrationDirs) {
	case 0:
		log.Println("No schema migration directory specified, skipping schema migrations")
	case 1:
		if err := migrateSchema(ctx, conf, c.SchemaMigrationDirs[0]); err != nil {
			return errors.Wrap(err, "migrateSchema()")
		}
	default:
		if err := linkAndMigrateDirs(ctx, conf, c.SchemaMigrationDirs, "schema"); err != nil {
			return err
		}
	}

	switch len(c.dataMigrationDirs) {
	case 0:
		log.Println("No Data Migration scripts provided. No changes applied.")
	case 1:
		if err := migrateData(ctx, conf, c.dataMigrationDirs[0]); err != nil {
			return errors.Wrap(err, "migrateData()")
		}
	default:
		if err := linkAndMigrateDirs(ctx, conf, c.dataMigrationDirs, "data"); err != nil {
			return err
		}
	}

	return nil
}

func linkAndMigrateDirs(ctx context.Context, conf *config, migrationSourceURLs []string, migrateType string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "os.Getwd()")
	}

	tempAllMigrationsDirPath, err := os.MkdirTemp(cwd, "all_migrations")
	if err != nil {
		return errors.Wrap(err, "os.MkdirTemp()")
	}
	defer func() {
		if err := os.RemoveAll(tempAllMigrationsDirPath); err != nil {
			log.Printf("error: %v\n", errors.Wrap(err, "os.RemoveAll()"))
		}
	}()

	for _, migrationSourceURL := range migrationSourceURLs {
		migrationDirClean := strings.TrimPrefix(migrationSourceURL, "file://")
		migrationDir, err := os.ReadDir(migrationDirClean)
		if err != nil {
			return errors.Wrap(err, "os.ReadDir()")
		}

		for _, dirEntry := range migrationDir {
			if dirEntry.IsDir() {
				continue
			}

			oldPath := filepath.Join(migrationDirClean, dirEntry.Name())
			newPath := filepath.Join(tempAllMigrationsDirPath, dirEntry.Name())

			if err := os.Link(oldPath, newPath); err != nil {
				return errors.Wrap(err, "os.Link()")
			}
		}
	}

	switch migrateType {
	case "schema":
		if err := migrateSchema(ctx, conf, fmt.Sprintf("file://%s", tempAllMigrationsDirPath)); err != nil {
			return errors.Wrap(err, "migrateSchema()")
		}

	case "data":
		if err := migrateData(ctx, conf, fmt.Sprintf("file://%s", tempAllMigrationsDirPath)); err != nil {
			return errors.Wrap(err, "migrateData()")
		}

	default:
		return errors.Newf("expected \"schema\" or \"data\" migration type, got %q", migrateType)
	}

	return nil
}

func migrateSchema(ctx context.Context, conf *config, migrationSourceURL string) error {
	log.Printf("Running bootstrap migrations with schema dir: %s \n", migrationSourceURL)
	if err := conf.migrateClient.MigrateUpSchema(ctx, migrationSourceURL); err != nil &&
		!errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to run schema migrations")
	} else if errors.Is(err, migrate.ErrNoChange) {
		log.Println("No new Migration scripts found. No changes applied.")
	} else {
		log.Println("Schema migrations successful")
	}

	return nil
}

func migrateData(ctx context.Context, conf *config, migrationSourceURL string) error {
	log.Println("Running bootstrap data migrations")
	if err := conf.migrateClient.MigrateUpData(ctx, migrationSourceURL); err != nil &&
		!errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to run data migrations")
	} else if errors.Is(err, migrate.ErrNoChange) {
		log.Println("No new Migration scripts found. No changes applied.")
	} else {
		log.Println("Data migrations successful")
	}

	return nil
}
