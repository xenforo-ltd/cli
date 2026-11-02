package worktree

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/ui"
)

// RemoveWorktree removes a worktree and, unless kept, its containers and
// volumes. Unless forced, it checks for work that would be lost before
// destroying the environment. Containers are removed before the checkout
// because Compose needs its files. Registry update failures are warnings.
func RemoveWorktree(ctx context.Context, name string, opts RemoveOptions) error {
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

	if !opts.Force {
		if err := checkRemovable(ctx, target); err != nil {
			return err
		}
	}

	if !opts.KeepContainers {
		if err := destroyWorktreeEnvironment(ctx, target); err != nil {
			return err
		}
	}

	if err := remove(ctx, source, target, opts.Force); err != nil {
		return err
	}

	if err := forgetWorktree(target); err != nil {
		warnRegistryUpdateFailed()

		if opts.Verbose {
			ui.PrintDetail(err.Error())
		}
	}

	ui.SuccessBox("Removed worktree "+ui.Path.Render(ui.ShortHome(target)), nil)

	return nil
}

// forgetWorktree removes the path from the worktree registry.
func forgetWorktree(path string) error {
	reg, err := newRegistry()
	if err != nil {
		return err
	}

	return reg.remove(path)
}
