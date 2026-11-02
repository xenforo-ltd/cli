// Package main provides the CLI entry point.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/xenforo-ltd/cli/internal/cli"
)

// main exits with the status returned by run.
func main() {
	os.Exit(run())
}

// run dispatches the CLI and records interrupt signals for conventional exit
// statuses: 130 for SIGINT and 143 for SIGTERM.
func run() int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(signals)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		cli.RecordInterruptSignal(<-signals)
		cancel()
	}()

	return cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}
