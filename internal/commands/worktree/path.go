package worktree

import (
	"context"
	"fmt"
	"os"
)

func Path(ctx context.Context, name string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := sourceCheckout(ctx, cwd)
	if err != nil {
		return err
	}

	target, err := locate(ctx, source, name)
	if err != nil {
		return err
	}

	// Printed bare, with no decoration, so it can be used directly in a shell.
	fmt.Println(target)

	return nil
}
