package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// errInvalidBranch indicates a branch name that cannot be used.
	errInvalidBranch = errors.New("invalid branch name")

	// errBranchExists indicates the branch is already present.
	errBranchExists = errors.New("branch already exists")

	// errWorktreeExists indicates the target directory is already in use.
	errWorktreeExists = errors.New("worktree already exists")

	// errWorktreeNotFound indicates no worktree matches a branch or name.
	errWorktreeNotFound = errors.New("worktree not found")

	// errNotXenForo indicates the source is not a XenForo installation.
	errNotXenForo = errors.New("not a XenForo directory")
)

// backendOptions describes the source, branch or Jujutsu workspace, directory
// name, base revision, and Docker instance for a worktree. The directory name
// defaults to a choice based on the branch, the base to the source's current
// revision, and the instance to the directory name.
type backendOptions struct {
	sourcePath string

	branch string

	name string

	base string

	instance string
}

// backendResult records the worktree path and branch, source path and branch,
// Docker instance, and creation time of a created worktree.
type backendResult struct {
	path string

	branch string

	sourcePath string

	sourceBranch string

	instance string

	createdAt time.Time
}

// preflight validates a request without changing anything.
//
// It does not mutate the repository or filesystem. The directory name must be
// a single safe path segment and its target must not already exist. A caller
// may already have reserved the name in the registry.
func preflight(ctx context.Context, b backend, sourcePath, branch, name string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("%w: branch name is empty", errInvalidBranch)
	}

	xfPath := filepath.Join(sourcePath, "src", "XF.php")
	if _, err := os.Stat(xfPath); err != nil {
		return fmt.Errorf("%w: src/XF.php not found in %s", errNotXenForo, sourcePath)
	}

	exists, err := b.nameExists(ctx, sourcePath, branch)
	if err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("%w: %s", errBranchExists, branch)
	}

	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: %q is not a usable worktree name", errInvalidBranch, name)
	}

	target := filepath.Join(worktreesDir(sourcePath), name)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%w: %s", errWorktreeExists, target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to inspect %s: %w", target, err)
	}

	return nil
}

// create makes a worktree on a new branch.
//
// It does not configure Docker or install anything; that is the caller's job.
// Pre-flight runs before the backend creates the checkout. A failed backend
// creation removes the parent directory if it is empty.
func create(ctx context.Context, opts backendOptions) (*backendResult, error) {
	b, source, err := resolveSource(ctx, opts.sourcePath)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(opts.branch) == "" {
		return nil, fmt.Errorf("%w: branch name is empty", errInvalidBranch)
	}

	name := opts.name
	if name == "" {
		name, err = chooseName(source, opts.branch, nil)
		if err != nil {
			return nil, err
		}
	}

	if err := preflight(ctx, b, source, opts.branch, name); err != nil {
		return nil, err
	}

	sourceBranch, err := b.currentName(ctx, source)
	if err != nil {
		return nil, err
	}

	target := filepath.Join(worktreesDir(source), name)

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	if err := b.create(ctx, source, opts.branch, opts.base, target); err != nil {
		_ = os.Remove(filepath.Dir(target))

		return nil, err
	}

	instance := opts.instance
	if instance == "" {
		instance = name
	}

	return &backendResult{
		path:         target,
		branch:       opts.branch,
		sourcePath:   source,
		sourceBranch: sourceBranch,
		instance:     instance,
		createdAt:    time.Now().UTC(),
	}, nil
}
