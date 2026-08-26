package stagedb

import (
	"context"
	"log"
	"os"
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
		Use:   "stagedb [target]",
		Short: "Back up and restore given source Spanner database to provided target database",
		Long:  "Backs up the configured source Spanner database and restores it to the provided target database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			target := args[0]
			if err := run(ctx, target, cmd); err != nil {
				return errors.Wrap(err, "run()")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&backupOnly, "backupOnly", "b", false, "Only execute backup")

	return cmd
}

func run(ctx context.Context, target string, cmd *cobra.Command) error {
	db, err := newConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize config")
	}
	defer func() {
		if err := db.spanner.Close(); err != nil {
			log.Println("error closing database")
		}
	}()

	backupOnly, err := cmd.Flags().GetBool("backupOnly")
	if err != nil {
		return errors.Wrap(err, "run()")
	}
	if backupOnly {
		log.Println("backup only flag received, will not restore")
		if err := backup(ctx, db, target); err != nil {
			return err
		}

		return nil
	}

	if err := backupRestore(ctx, db, target); err != nil {
		return err
	}

	return nil
}

func backup(ctx context.Context, db *config, target string) error {
	_, err := db.spanner.Backup(ctx)
	if err != nil {
		return errors.Wrap(err, "backup()")
	}

	return nil
}

func backupRestore(ctx context.Context, db *config, target string) error {
	normalizedTarget := strings.ToLower(target)
	if strings.Contains(normalizedTarget, "prd") || strings.Contains(normalizedTarget, "prod") {
		return errors.Newf("will not target a production database. target: %s", normalizedTarget)
	}
	sourceDb, ok := os.LookupEnv("GOOGLE_CLOUD_SPANNER_DATABASE_NAME")
	if !ok {
		return errors.New("GOOGLE_CLOUD_SPANNER_DATABASE_NAME required environment variable not found.")
	}
	log.Printf("source database set via environment variable: %s", sourceDb)

	backup, err := db.spanner.Backup(ctx)
	if err != nil {
		return errors.Wrap(err, "Backup()")
	}

	if err := db.spanner.Restore(ctx, backup, normalizedTarget); err != nil {
		return errors.Wrap(err, "Restore()")
	}

	return nil
}
