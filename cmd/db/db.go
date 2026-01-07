package db

import (
	"context"

	"github.com/cccteam/deployment-tools/cmd/db/spanner"
	"github.com/spf13/cobra"
)

type command struct{}

// Command returns the configured command
func Command(ctx context.Context) *cobra.Command {
	cli := command{}

	return cli.Setup(ctx)
}

func (command) Setup(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Commands for database operations during a deployment",
		Long:  "Commands for database operations during a deployment, such as bootstrapping and dropping schema",
	}

	cmd.AddCommand(spanner.Command(ctx))

	return cmd
}
