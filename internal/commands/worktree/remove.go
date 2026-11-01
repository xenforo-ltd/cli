package worktree

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/ui"
)

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

	// The safety check runs before anything is destroyed. Tearing down first
	// and refusing afterwards would report that the worktree was kept while
	// its database and volumes had already been deleted.
	if !opts.Force {
		if err := checkRemovable(ctx, target); err != nil {
			return err
		}
	}

	// Containers must be torn down before the directory goes: compose reads
	// compose.yaml from the worktree to know what it owns, so removing the
	// files first would strand the containers and volumes.
	if !opts.KeepContainers {
		if err := destroyWorktreeEnvironment(ctx, target); err != nil {
			return err
		}
	}

	if err := remove(ctx, source, target, opts.Force); err != nil {
		return err
	}

	if err := forgetWorktree(target); err != nil {
		// Reported rather than ignored: the worktree is gone but the registry
		// still lists it, and only this message tells the user why.
		warnRegistryUpdateFailed()

		if opts.Verbose {
			ui.PrintDetail(err.Error())
		}
	}

	ui.SuccessBox("Removed worktree "+ui.Path.Render(ui.ShortHome(target)), nil)

	return nil
}

func forgetWorktree(path string) error {
	reg, err := newRegistry()
	if err != nil {
		return err
	}

	return reg.remove(path)
}
