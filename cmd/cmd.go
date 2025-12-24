/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	"github.com/cccteam/deployment-tools/cmd/bootstrap"
	"github.com/cccteam/deployment-tools/cmd/resolvedeployment"
	"github.com/go-playground/errors/v5"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "deployment-tools",
	Short: "A command line to to be used for executing different actions during a deployment process",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ctx context.Context) error {
	rootCmd.AddCommand(resolvedeployment.Command(ctx))
	rootCmd.AddCommand(bootstrap.Command(ctx))

	err := rootCmd.Execute()
	if err != nil {
		return errors.Wrap(err, "rootCmd.Execute()")
	}

	return nil
}
