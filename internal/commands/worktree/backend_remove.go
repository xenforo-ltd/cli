package worktree

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/environment"
)

var (
	// ErrDirtyWorktree indicates uncommitted changes would be lost.
	errDirtyWorktree = errors.New("worktree has uncommitted changes")

	// ErrUnmergedCommits indicates commits exist only in this worktree.
	errUnmergedCommits = errors.New("worktree has commits not present on any remote")
)

// worktreeStatus describes the state of a worktree's working tree and branch.
type worktreeStatus struct {
	// Modified lists paths with uncommitted changes, including untracked files.
	Modified []string

	// UnmergedCommits lists commits not reachable from any remote branch.
	UnmergedCommits []string
}

// checkRemovable reports whether a worktree can be removed without losing work.
//
// It is separate from Remove so callers can run the check before destroying
// anything the removal depends on. Tearing down containers and volumes first
// and only then discovering that the worktree is dirty would refuse the
// removal after the data it was protecting had already been deleted.
func checkRemovable(ctx context.Context, worktreePath string) error {
	b, err := detectBackend(ctx, worktreePath)
	if err != nil {
		return err
	}

	status, err := b.status(ctx, worktreePath)
	if err != nil {
		return err
	}

	return removableError(status)
}

// removableError reports the work a removal would lose, if any.
func removableError(status worktreeStatus) error {
	if len(status.Modified) > 0 {
		return fmt.Errorf("%w:\n  %s", errDirtyWorktree, strings.Join(status.Modified, "\n  "))
	}

	if len(status.UnmergedCommits) > 0 {
		return fmt.Errorf("%w:\n  %s", errUnmergedCommits, strings.Join(status.UnmergedCommits, "\n  "))
	}

	return nil
}

// remove deletes a worktree and its branch.
//
// Unless force is set, it refuses when the worktree holds uncommitted changes
// or commits that exist nowhere else, listing what would be lost. Removing
// containers and volumes is the caller's responsibility.
func remove(ctx context.Context, sourcePath, worktreePath string, force bool) error {
	b, err := detectBackend(ctx, sourcePath)
	if err != nil {
		return err
	}

	if !force {
		status, err := b.status(ctx, worktreePath)
		if err != nil {
			return err
		}

		if err := removableError(status); err != nil {
			return err
		}
	}

	return b.remove(ctx, sourcePath, worktreePath, force)
}

// generatedPaths returns the files "worktree create" writes into a worktree.
//
// The set comes from the embedded Docker tree, so it tracks whatever
// extraction actually writes instead of a hand-maintained list that would
// quietly fall out of date. Both backends treat these as generated rather than
// as work the user would lose.
func generatedPaths() (map[string]bool, error) {
	files, err := environment.ListEmbeddedFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to determine generated files: %w", err)
	}

	paths := make(map[string]bool, len(files))
	for _, file := range files {
		// git and jj report paths with forward slashes on every platform.
		paths[filepath.ToSlash(file)] = true
	}

	return paths, nil
}
