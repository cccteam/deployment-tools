package stagedb

import (
	"context"
	"log"
	"strings"

	"github.com/go-playground/errors/v5"
	"github.com/spf13/cobra"
)

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := setup(ctx)

	return cli
}

func setup(ctx context.Context) *cobra.Command {
	var backupOnly bool
	cmd := &cobra.Command{
		Use:   "stagedb <target>",
		Short: "Back up and restore given source Spanner database to provided target database",
		Long:  "Backs up the configured source Spanner database and restores it to the provided target database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			target := args[0]
			if err := run(ctx, target, backupOnly); err != nil {
				return errors.Wrap(err, "run()")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&backupOnly, "backup-only", "b", false, "Only execute backup")

	return cmd
}

func run(ctx context.Context, target string, backupOnly bool) error {
	db, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}

	defer func() {
		if err := db.spanner.Close(); err != nil {
			log.Printf("error closing database: %v\n", err)
		}
	}()

	normalizedTarget := normalizeName(target)

	if backupOnly {
		log.Printf("backup only flag received, will not restore\nBacking up database %s", db.spanner.SourceDb)
		if err := backup(ctx, db); err != nil {
			return err
		}

		return nil
	}

	log.Printf("restoring database %s database\n", target)
	if err := backupRestore(ctx, db, normalizedTarget); err != nil {
		return err
	}

	return nil
}

func backup(ctx context.Context, db *config) error {
	_, err := db.spanner.Backup(ctx)
	if err != nil {
		return errors.Wrap(err, "db.spanner.Backup()")
	}

	return nil
}

func normalizeName(target string) string {
	return strings.ToLower(target)
}

func backupRestore(ctx context.Context, db *config, target string) error {
	backup, err := db.spanner.Backup(ctx)
	if err != nil {
		return errors.Wrap(err, "Backup()")
	}

	if err := db.spanner.Restore(ctx, backup, target); err != nil {
		return errors.Wrap(err, "Restore()")
	}

	return nil
}
