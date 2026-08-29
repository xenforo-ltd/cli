// Package main provides the CLI entry point.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// NotifyContext reports only that the context ended, not which signal
	// ended it, so the signal is recorded here to exit with its conventional
	// 128+signum status: 130 for SIGINT, 143 for SIGTERM.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(signals)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig, ok := <-signals
		if !ok {
			return
		}

		recordInterruptSignal(sig)
		cancel()
	}()

	Execute(ctx)
}
