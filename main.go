/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"context"
	"log"

	"github.com/cccteam/deployment-tools/cmd"
	"github.com/jtwatson/shutdown"
)

func main() {
	ctx := context.Background()
	if err := execute(ctx); err != nil {
		log.Fatal(err)
	}
}

func execute(ctx context.Context) error {
	ctx, cancel := shutdown.CaptureInterrupts(ctx)
	defer cancel()

	if err := cmd.Execute(ctx); err != nil {
		return err
	}

	return nil
}
