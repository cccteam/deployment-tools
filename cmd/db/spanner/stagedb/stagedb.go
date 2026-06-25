package stagedb

import (
	"context"

	"github.com/go-playground/errors/v5"
	"github.com/spf13/cobra"
)

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := command{}

	return cli.Setup(ctx)
}

type command struct {
	sourceDatabase string
	targetDatabase string
}

func (c *command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage-db [source] [destination]",
		Short: "Runs backup/restore on database",
		Long:  "Runs backup/restore on database",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			source, destination := args[0], args[1]
			if err := c.Run(ctx, cmd, source, destination); err != nil {
				return errors.Wrap(err, "command.Run()")
			}

			return nil
		},
	}

	return cmd
}

func (c *command) ValidateFlags(cmd *cobra.Command) error {
	return nil
}

func (c *command) Run(ctx context.Context, cmd *cobra.Command, source, destination string) error {
	db, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer db.spanner.Close()

	if err = db.spanner.BackupRestore(ctx, source, destination); err != nil {
		return err
	}

	return nil
}
