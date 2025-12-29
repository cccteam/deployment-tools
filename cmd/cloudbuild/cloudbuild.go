package cloudbuild

import (
	"context"

	"github.com/cccteam/deployment-tools/cmd/cloudbuild/resolvedeployment"
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
		Use:   "cloudbuild",
		Short: "Commands for executing",
		Long:  "Commands for google cloud build operations during a deployment, such as resolving deployments",
	}

	cmd.AddCommand(resolvedeployment.Command(ctx))

	return cmd
}
