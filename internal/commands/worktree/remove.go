package worktree

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/ui"
)

// RemoveWorktree removes a worktree and, unless kept, its containers and
// volumes. It first confirms that the target is a linked, non-source checkout
// of the repository, even when forced, and refuses to tear down an environment
// that shares the source's instance. Unless forced, it checks for work that
// would be lost before destroying the environment. Containers are removed
// before the checkout because Compose needs its files. Registry update failures are warnings.
func RemoveWorktree(ctx context.Context, name string, opts RemoveOptions) error {
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

	worktreePath := found.path

	expected := name
	if found.byName {
		expected = ""
	}

	if err := validateRemovalTarget(ctx, source, worktreePath, expected); err != nil {
		return err
	}

	if !opts.Force {
		if err := checkRemovable(ctx, worktreePath); err != nil {
			return err
		}
	}

	if !opts.KeepContainers {
		if err := refuseSharedEnvironment(source, worktreePath); err != nil {
			return err
		}

		if err := destroyWorktreeEnvironment(ctx, worktreePath); err != nil {
			return err
		}
	}

	if err := remove(ctx, source, worktreePath, opts.Force); err != nil {
		return err
	}

	if err := forgetWorktree(worktreePath); err != nil {
		warnRegistryUpdateFailed()

		if opts.Verbose {
			ui.PrintDetail(err.Error())
		}
	}

	ui.SuccessBox("Removed worktree "+ui.Path.Render(ui.ShortHome(worktreePath)), nil)

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
