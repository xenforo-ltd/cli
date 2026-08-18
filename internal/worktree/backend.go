package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotARepository indicates a path is not inside a supported repository.
var ErrNotARepository = errors.New("not a git or Jujutsu repository")

// repositoryKind identifies the version control system a directory belongs to.
type repositoryKind int

const (
	// repositoryNone means no repository marker was found.
	repositoryNone repositoryKind = iota

	// repositoryGit means a .git marker was found.
	repositoryGit

	// repositoryJujutsu means a .jj marker was found.
	repositoryJujutsu
)

// repositoryMarker returns the version control system whose marker is closest
// to dir, walking from dir to its ancestors.
//
// Jujutsu is checked before Git at every level so a colocated repository -- one
// carrying both .jj and .git metadata -- is treated as Jujutsu. Detection is
// structural: the marker is only located, never interpreted, so a missing or
// misconfigured VCS tool cannot make a repository look like a different one.
// The marker is matched as a file or a directory because a linked Git worktree
// records .git as a file pointing at the shared repository.
func repositoryMarker(ctx context.Context, dir string) (repositoryKind, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return repositoryNone, fmt.Errorf("failed to resolve repository path %s: %w", dir, err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return repositoryNone, err
		}

		exists, err := markerExists(filepath.Join(abs, ".jj"))
		if err != nil {
			return repositoryNone, err
		}

		if exists {
			return repositoryJujutsu, nil
		}

		exists, err = markerExists(filepath.Join(abs, ".git"))
		if err != nil {
			return repositoryNone, err
		}

		if exists {
			return repositoryGit, nil
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return repositoryNone, nil
		}

		abs = parent
	}
}

// markerExists reports whether path exists, propagating inspection failures
// other than "not found" so a permission or I/O error is never mistaken for an
// absent repository.
func markerExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("failed to inspect %s: %w", path, err)
}

// backend is the version control system seam behind worktree orchestration.
//
// Every method is a single VCS operation that creating, inspecting or removing
// a checkout needs. Keeping the interface private lets the package present one
// stable, VCS-neutral API -- Create, Status, Remove and the rest -- while Git
// and Jujutsu each implement the operations in their own terms: a Git branch
// and worktree, or a Jujutsu workspace.
type backend interface {
	// sourceCheckout returns the canonical source checkout containing dir. For
	// a linked checkout it returns the original checkout, so new checkouts are
	// siblings of the source rather than nested inside the current one.
	sourceCheckout(ctx context.Context, dir string) (string, error)

	// nameExists reports whether a branch (Git) or workspace (Jujutsu) named
	// name already exists in the repository containing repoDir.
	nameExists(ctx context.Context, repoDir, name string) (bool, error)

	// currentName returns the branch (Git) or workspace (Jujutsu) checked out
	// at checkoutPath.
	currentName(ctx context.Context, checkoutPath string) (string, error)

	// owner returns the branch or workspace occupying checkoutPath, or an
	// empty string when nothing is registered there.
	owner(ctx context.Context, repoDir, checkoutPath string) (string, error)

	// create checks name out at target, based on base when base is non-empty.
	// It is a VCS mutation only: validating and creating paths is the caller's
	// job.
	create(ctx context.Context, source, name, base, target string) error

	// status inspects checkoutPath for work that removal would lose.
	status(ctx context.Context, checkoutPath string) (WorktreeStatus, error)

	// remove deletes the checkout at checkoutPath and its branch or workspace.
	// force only widens what the VCS will discard; the shared pre-removal
	// checks are the caller's responsibility.
	remove(ctx context.Context, source, checkoutPath string, force bool) error
}

// detectBackend selects the backend for the repository containing dir.
//
// The repository's metadata decides the backend: .jj means Jujutsu, including
// in a repository colocated with Git, and .git means Git. Once a marker is
// found the matching backend is returned unconditionally, so a repository whose
// tooling is missing or broken surfaces that failure from the operation instead
// of quietly falling through to the other VCS. A directory with neither marker
// yields the repository error.
func detectBackend(ctx context.Context, dir string) (backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	kind, err := repositoryMarker(ctx, dir)
	if err != nil {
		return nil, err
	}

	switch kind {
	case repositoryJujutsu:
		return jjBackend{}, nil
	case repositoryGit:
		return gitBackend{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrNotARepository, dir)
	}
}

// resolveSource detects the backend for dir and resolves the canonical source
// checkout, so a single operation probes the VCS once and reuses the backend.
func resolveSource(ctx context.Context, dir string) (backend, string, error) {
	b, err := detectBackend(ctx, dir)
	if err != nil {
		return nil, "", err
	}

	source, err := b.sourceCheckout(ctx, dir)
	if err != nil {
		return nil, "", err
	}

	return b, source, nil
}

// SourceCheckout returns the main checkout for the repository containing dir.
//
// When dir is inside a linked worktree or workspace this returns the *original*
// checkout, not the linked one. That keeps new checkouts siblings of the source
// rather than nesting them, so running the command from within one behaves the
// same as running it from the source.
func SourceCheckout(ctx context.Context, dir string) (string, error) {
	_, source, err := resolveSource(ctx, dir)

	return source, err
}

// BranchExists reports whether a local branch of the given name exists.
//
// For a Jujutsu repository the argument is a workspace name.
func BranchExists(ctx context.Context, repoDir, branch string) (bool, error) {
	b, err := detectBackend(ctx, repoDir)
	if err != nil {
		return false, err
	}

	return b.nameExists(ctx, repoDir, branch)
}

// CurrentBranch returns the branch checked out in repoDir.
//
// For a Jujutsu repository this returns the workspace name.
func CurrentBranch(ctx context.Context, repoDir string) (string, error) {
	b, err := detectBackend(ctx, repoDir)
	if err != nil {
		return "", err
	}

	return b.currentName(ctx, repoDir)
}

// worktreeOwner returns the branch or workspace occupying worktreePath, or an
// empty string when nothing is registered there.
//
// This makes a collision actionable: the user is told which branch already owns
// the directory, rather than only that something does.
func worktreeOwner(ctx context.Context, repoDir, worktreePath string) (string, error) {
	b, err := detectBackend(ctx, repoDir)
	if err != nil {
		return "", err
	}

	return b.owner(ctx, repoDir, worktreePath)
}
