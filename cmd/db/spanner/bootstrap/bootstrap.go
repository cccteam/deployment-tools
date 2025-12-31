package bootstrap

import (
	"context"
	"log"
	"strings"

	"github.com/go-playground/errors/v5"
	"github.com/spf13/cobra"
	"github.com/zredinger-ccc/migrate/v4"
	_ "github.com/zredinger-ccc/migrate/v4/source/file" // up/down script file source driver for the migrate package
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
				log.Fatal(err)
			}

			return nil
		},
	}

	cmd.Flags().
		StringVarP(&c.SchemaMigrationDir, "schema-dir", "s", "file://schema/migrations", "Directory containing schema migration files, using the file URI syntax")
	cmd.Flags().
		StringSliceVar(&c.dataMigrationDirs, "data-dirs", []string{"file://bootstrap/testdata"}, "Directories containing data migration files, using the file URI syntax")

	return cmd
}

func (c *command) ValidateFlags(cmd *cobra.Command) error {
	if len(c.dataMigrationDirs) == 0 {
		return errors.New("at least one data-dir flag is required")
	}

	return nil
}

func (c *command) Run(ctx context.Context, cmd *cobra.Command) error {
	conf, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer conf.close()

	if c.SchemaMigrationDir != "" {
		log.Printf("Running bootstrap migrations with schema dir: %s \n", c.SchemaMigrationDir)
		if err := conf.migrateClient.MigrateUpSchema(ctx, c.SchemaMigrationDir); err != nil &&
			!errors.Is(err, migrate.ErrNoChange) {
			return errors.Wrap(err, "failed to run schema migrations")
		}
		log.Println("Schema migrations successful")
	} else {
		log.Println("No schema migration directory specified, skipping schema migrations")
	}

	log.Printf("Running bootstrap data migrations [%s]", strings.Join(c.dataMigrationDirs, ", "))
	if err := conf.migrateClient.MigrateUpData(ctx, c.dataMigrationDirs...); err != nil &&
		!errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to failed to run migrations")
	} else if errors.Is(err, migrate.ErrNoChange) {
		log.Println("No new Migration scripts found. No changes applied.")
	} else {
		log.Println("Ran data migrations successfully")
	}

	return nil
}
