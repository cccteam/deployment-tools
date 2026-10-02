package main

import (
	"context"
	"log"

	"github.com/cccteam/deployment-tools/cmd"
	"github.com/go-playground/errors/v5"
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
		return errors.Wrap(err, "cmd.Execute()")
	}

	return nil
}
