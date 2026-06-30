package stagedb

import (
	"context"
	"log"

	"github.com/go-playground/errors/v5"
	"github.com/spf13/cobra"
)

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := setup(ctx)

	return cli
}

func setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage-db [target]",
		Short: "Runs backup/restore on database",
		Long:  "Runs backup/restore on database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			target := args[0]
			if err := run(ctx, target); err != nil {
				return errors.Wrap(err, "command.Run()")
			}

			return nil
		},
	}

	return cmd
}

func run(ctx context.Context, target string) error {
	db, err := newConfig(ctx, target)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer db.spanner.Close()

	if err := backupRestore(ctx, db, target); err != nil {
		return err
	}

	return nil
}

func backupRestore(ctx context.Context, db *config, destination string) error {
	backup, err := db.spanner.Backup(ctx)
	if err != nil {
		log.Println("error backing up ", err)

		return errors.Wrap(err, "Backup()")
	}

	if err := db.spanner.Restore(ctx, backup, destination); err != nil {
		return errors.Wrap(err, "Restore()")
	}

	return nil
}
