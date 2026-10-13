package worktree

import (
	"context"
	"fmt"
	"os"
)

// List prints registered worktrees for the current source checkout.
func List(ctx context.Context, json bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := managementSource(ctx, cwd)
	if err != nil {
		return err
	}

	registry, err := newRegistry()
	if err != nil {
		return err
	}

	entries, err := registry.forSource(source)
	if err != nil {
		return registryReadError(err)
	}

	return printWorktrees(entries, json)
}
