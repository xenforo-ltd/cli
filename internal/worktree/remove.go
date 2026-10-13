package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/docker"
)

var (
	// ErrDirtyWorktree indicates uncommitted changes would be lost.
	ErrDirtyWorktree = errors.New("worktree has uncommitted changes")

	// ErrUnmergedCommits indicates commits exist only in this worktree.
	ErrUnmergedCommits = errors.New("worktree has commits not present on any remote")
)

// WorktreeStatus describes the state of a worktree's working tree and branch.
type WorktreeStatus struct {
	// Modified lists paths with uncommitted changes, including untracked files.
	Modified []string

	// UnmergedCommits lists commits not reachable from any remote branch.
	UnmergedCommits []string
}

// CheckRemovable reports whether a worktree can be removed without losing work.
//
// It is separate from Remove so callers can run the check before destroying
// anything the removal depends on. Tearing down containers and volumes first
// and only then discovering that the worktree is dirty would refuse the
// removal after the data it was protecting had already been deleted.
func CheckRemovable(ctx context.Context, worktreePath string) error {
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
func removableError(status WorktreeStatus) error {
	if len(status.Modified) > 0 {
		return fmt.Errorf("%w:\n  %s", ErrDirtyWorktree, strings.Join(status.Modified, "\n  "))
	}

	if len(status.UnmergedCommits) > 0 {
		return fmt.Errorf("%w:\n  %s", ErrUnmergedCommits, strings.Join(status.UnmergedCommits, "\n  "))
	}

	return nil
}

// ValidateRemovalTarget confirms that target may be removed as a linked
// checkout of source.
//
// A registry record is a discovery aid, not authority: it may be stale or
// tampered with. Before anything destructive runs, the target must exist as a
// directory, belong to the same kind of repository as the source, be the exact
// root of a checkout that source's repository registers, not be the source
// itself, and be owned by a branch or workspace. When expectedName is
// non-empty it must match the owner, so a same-named checkout in another
// repository cannot be selected through a stale record.
//
// Every check is read-only, and all of them hold even when removal is forced:
// force widens what the version control system will discard, never whose
// checkout is being deleted.
func ValidateRemovalTarget(ctx context.Context, source, target, expectedName string) error {
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no checkout at %s: %w", target, err)
		}

		return fmt.Errorf("failed to inspect %s: %w", target, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", target)
	}

	sourceKind, err := repositoryMarker(ctx, source)
	if err != nil {
		return err
	}

	targetKind, err := repositoryMarker(ctx, target)
	if err != nil {
		return err
	}

	if sourceKind != targetKind {
		return fmt.Errorf("%s is not part of the repository at %s", target, source)
	}

	b, err := detectBackend(ctx, source)
	if err != nil {
		return err
	}

	discovered, err := b.linkedCheckout(ctx, target)
	if err != nil {
		return err
	}

	if resolveSymlinks(discovered.root) != resolveSymlinks(target) {
		return fmt.Errorf("%s is not the root of a registered checkout", target)
	}

	// The supplied source is authoritative: a Jujutsu workspace that is not the
	// repository host has itself as its discovered root, so comparing against
	// the discovered source alone would let a stale record or a symlink select
	// the very checkout removal was invoked against. Compare the target with
	// the supplied source before anything is destroyed.
	if resolveSymlinks(target) == resolveSymlinks(source) {
		return fmt.Errorf("refusing to remove the source checkout %s", target)
	}

	// A target that is the repository host or main checkout is refused even
	// when it was not the supplied source, so a stale record cannot name
	// another repository's primary checkout.
	if resolveSymlinks(discovered.source) == resolveSymlinks(discovered.root) {
		return fmt.Errorf("refusing to remove the source checkout %s", target)
	}

	ownerName, err := owner(ctx, b, source, target)
	if err != nil {
		return err
	}

	if ownerName == "" {
		return fmt.Errorf("no branch or workspace is registered at %s in the repository at %s", target, source)
	}

	if expectedName != "" && ownerName != expectedName {
		return fmt.Errorf("%s has %s checked out, not %s: refusing to remove it", target, ownerName, expectedName)
	}

	return nil
}

// Remove deletes a worktree and its branch.
//
// Unless force is set, it refuses when the worktree holds uncommitted changes
// or commits that exist nowhere else, listing what would be lost. Removing
// containers and volumes is the caller's responsibility.
//
// The target is validated first -- it must be a registered, non-source
// checkout of sourcePath -- so a stale registry record cannot redirect the
// removal at an unrelated directory.
func Remove(ctx context.Context, sourcePath, worktreePath string, force bool) error {
	if err := ValidateRemovalTarget(ctx, sourcePath, worktreePath, ""); err != nil {
		return err
	}

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
	files, err := docker.ListEmbeddedFiles()
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
