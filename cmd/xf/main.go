// Package main provides the CLI entry point.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/xenforo-ltd/cli/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	// NotifyContext reports only that the context ended, not which signal
	// ended it, so the signal is recorded here to exit with its conventional
	// 128+signum status: 130 for SIGINT, 143 for SIGTERM.
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
