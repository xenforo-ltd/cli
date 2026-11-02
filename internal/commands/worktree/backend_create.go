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

// backendOptions describes a worktree to create.
type backendOptions struct {
	// sourcePath is the checkout to branch from.
	sourcePath string

	// branch is the branch (Git) or workspace (Jujutsu) to create in the new
	// checkout.
	branch string

	// name names the new directory. Defaults to chooseName's choice for
	// branch.
	name string

	// base is the revision to base the new checkout on, interpreted with the
	// selected backend's revision syntax. Defaults to the source's current HEAD.
	base string

	// instance overrides the Docker instance name, which defaults to name.
	instance string
}

// backendResult describes a created worktree.
type backendResult struct {
	// path is the worktree location.
	path string

	// branch is the branch checked out in the worktree.
	branch string

	// sourcePath is the checkout it was created from.
	sourcePath string

	// sourceBranch is the branch the source was on at creation time.
	sourceBranch string

	// instance is the Docker instance name for the worktree.
	instance string

	// createdAt is when the worktree was created.
	createdAt time.Time
}

// preflight validates a request without changing anything.
//
// It runs before any mutation so that a rejected request leaves no partial
// state behind: no directory, no branch, no registry entry.
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

	// The name becomes a path segment, so it must not be able to name a
	// different directory.
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: %q is not a usable worktree name", errInvalidBranch, name)
	}

	// chooseName skips occupied directories, but a name supplied by the caller
	// has not been checked, and something can appear between the two.
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
// Pre-flight runs first, so a failure here leaves the source checkout untouched.
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
		// Leave no empty container directory behind after a failure.
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
