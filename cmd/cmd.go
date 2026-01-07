package cmd

import (
	"context"

	"github.com/cccteam/deployment-tools/cmd/db"
	"github.com/go-playground/errors/v5"
	"github.com/spf13/cobra"
)

// Execute configures the root command for the application and executes it
func Execute(ctx context.Context) error {
	cmd := &cobra.Command{
		Use:   "deployment-tools",
		Short: "A command line to to be used for executing different actions during a deployment process",
	}

	cmd.AddCommand(db.Command(ctx))

	if err := cmd.Execute(); err != nil {
		return errors.Wrap(err, "cmd.Execute()")
	}

	return nil
}
