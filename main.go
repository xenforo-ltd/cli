// Package main provides the CLI entry point.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/xenforo-ltd/cli/cmd"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cmd.Execute(ctx)
}
