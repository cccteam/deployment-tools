package bootstrap

import (
	"context"
	"fmt"
	"log"

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
}

// Setup returns the configured cli command
func (c *command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap database",
		Long:  "Bootstrap database by running specified migrations",
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			if err := c.ValidateFlags(); err != nil {
				return err
			}

			if err := c.Run(ctx, cmd); err != nil {
				log.Println(err)
			}

			return nil
		},
	}

	cmd.Flags().StringP("migrate-dir", "d", "file://bootstrap/testdata", "Directory containing migration files")

	return cmd
}

// ValidateFlags validates and processes any input flags
func (c *command) ValidateFlags() error {
	return nil
}

// Run executes the command
func (c *command) Run(ctx context.Context, cmd *cobra.Command) error {
	conf, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer conf.close()

	if err := conf.migrateClient.MigrateUp("file://" + cmd.Flags().Lookup("migrate-dir").Value.String()); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errors.Wrap(err, "failed to failed to run migrations")
	} else if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("No new Migration scripts found. No changes applied.")
	} else {
		fmt.Println("Ran migration successful")
	}

	return nil
}
