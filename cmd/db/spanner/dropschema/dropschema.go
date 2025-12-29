package dropschema

import (
	"context"
	"log"

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
	SchemaMigrationDir string
}

// Setup returns the configured cli command
func (c *command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drop",
		Short: "drop database tables",
		Long:  "Drop all database tables",
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

	return cmd
}

// ValidateFlags validates and processes any input flags
func (c *command) ValidateFlags(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (c *command) Run(ctx context.Context, cmd *cobra.Command) error {
	conf, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer conf.close()

	log.Println("Dropping schema tables...")

	if err := conf.migrateClient.MigrateDropSchema(ctx, c.SchemaMigrationDir); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to drop schema")
	}

	log.Println("Schema tables dropped successfully")

	return nil
}
