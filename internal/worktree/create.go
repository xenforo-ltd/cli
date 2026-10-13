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
	// ErrInvalidBranch indicates a branch name that cannot be used.
	ErrInvalidBranch = errors.New("invalid branch name")

	// ErrBranchExists indicates the branch is already present.
	ErrBranchExists = errors.New("branch already exists")

	// ErrWorktreeExists indicates the target directory is already in use.
	ErrWorktreeExists = errors.New("worktree already exists")

	// ErrWorktreeNotFound indicates no worktree matches a branch or name.
	ErrWorktreeNotFound = errors.New("worktree not found")

	// ErrNotXenForo indicates the source is not a XenForo installation.
	ErrNotXenForo = errors.New("not a XenForo directory")
)

// Options describes a worktree to create.
type Options struct {
	// SourcePath is the checkout to branch from.
	SourcePath string

	// Branch is the branch (Git) or workspace (Jujutsu) to create in the new
	// checkout.
	Branch string

	// Name names the new directory. Defaults to ChooseName's choice for
	// Branch.
	Name string

	// Base is the revision to base the new checkout on, interpreted with the
	// selected backend's revision syntax. Defaults to the source's current HEAD.
	Base string

	// Instance overrides the Docker instance name, which defaults to Name.
	Instance string
}

// Result describes a created or adopted worktree.
type Result struct {
	// Path is the worktree location.
	Path string

	// Branch is the branch or workspace checked out in the worktree.
	Branch string

	// SourcePath is the checkout that manages the worktree.
	SourcePath string

	// SourceBranch is the branch or workspace the source had at creation or
	// adoption time.
	SourceBranch string

	// Instance is the Docker instance name for the worktree.
	Instance string

	// CreatedAt is when the worktree was created, or last adopted.
	CreatedAt time.Time
}

// preflight validates a request without changing anything.
//
// It runs before any mutation so that a rejected request leaves no partial
// state behind: no directory, no branch, no registry entry.
func preflight(ctx context.Context, b backend, sourcePath, branch, name string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("%w: branch name is empty", ErrInvalidBranch)
	}

	xfPath := filepath.Join(sourcePath, "src", "XF.php")
	if _, err := os.Stat(xfPath); err != nil {
		return fmt.Errorf("%w: src/XF.php not found in %s", ErrNotXenForo, sourcePath)
	}

	exists, err := b.nameExists(ctx, sourcePath, branch)
	if err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("%w: %s", ErrBranchExists, branch)
	}

	// The name becomes a path segment, so it must not be able to name a
	// different directory.
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: %q is not a usable worktree name", ErrInvalidBranch, name)
	}

	// ChooseName skips occupied directories, but a name supplied by the caller
	// has not been checked, and something can appear between the two.
	target := filepath.Join(WorktreesDir(sourcePath), name)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%w: %s", ErrWorktreeExists, target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to inspect %s: %w", target, err)
	}

	return nil
}

// Create makes a worktree on a new branch.
//
// It does not configure Docker or install anything; that is the caller's job.
// Pre-flight runs first, so a failure here leaves the source checkout untouched.
func Create(ctx context.Context, opts Options) (*Result, error) {
	b, source, err := resolveSource(ctx, opts.SourcePath)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(opts.Branch) == "" {
		return nil, fmt.Errorf("%w: branch name is empty", ErrInvalidBranch)
	}

	name := opts.Name
	if name == "" {
		name, err = ChooseName(source, opts.Branch, nil)
		if err != nil {
			return nil, err
		}
	}

	if err := preflight(ctx, b, source, opts.Branch, name); err != nil {
		return nil, err
	}

	sourceBranch, err := b.currentName(ctx, source)
	if err != nil {
		return nil, err
	}

	target := filepath.Join(WorktreesDir(source), name)

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	if err := b.create(ctx, source, opts.Branch, opts.Base, target); err != nil {
		// Leave no empty container directory behind after a failure.
		_ = os.Remove(filepath.Dir(target))

		return nil, err
	}

	instance := opts.Instance
	if instance == "" {
		instance = name
	}

	return &Result{
		Path:         target,
		Branch:       opts.Branch,
		SourcePath:   source,
		SourceBranch: sourceBranch,
		Instance:     instance,
		CreatedAt:    time.Now().UTC(),
	}, nil
}
