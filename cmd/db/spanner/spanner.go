package spanner

import (
	"context"

	"github.com/cccteam/deployment-tools/cmd/db/spanner/bootstrap"
	"github.com/cccteam/deployment-tools/cmd/db/spanner/dropschema"
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
		Use:   "spanner",
		Short: "Commands for spanner database operations during a deployment",
		Long:  "Commands for spanner database operations during a deployment, such as bootstrapping and dropping schema",
	}

	cmd.AddCommand(bootstrap.Command(ctx))
	cmd.AddCommand(dropschema.Command(ctx))

	return cmd
}
