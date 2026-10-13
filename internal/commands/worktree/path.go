package worktree

import (
	"context"
	"fmt"
	"os"
)

// Path prints the named worktree's path without decoration for shell use.
func Path(ctx context.Context, name string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := managementSource(ctx, cwd)
	if err != nil {
		return err
	}

	found, err := findWorktree(ctx, source, name)
	if err != nil {
		return err
	}

	fmt.Println(found.path)

	return nil
}
