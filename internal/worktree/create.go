package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/xf"
)

var (
	// ErrInvalidBranch indicates a branch name that cannot be used.
	ErrInvalidBranch = errors.New("invalid branch name")

	// ErrBranchExists indicates the branch is already present.
	ErrBranchExists = errors.New("branch already exists")

	// ErrWorktreeExists indicates the target directory is already in use.
	ErrWorktreeExists = errors.New("worktree already exists")

	// ErrNotXenForo indicates the source is not a XenForo installation.
	ErrNotXenForo = errors.New("not a XenForo directory")
)

// Options describes a worktree to create.
type Options struct {
	// SourcePath is the checkout to branch from.
	SourcePath string

	// Branch is the branch to create in the new worktree.
	Branch string

	// Base is the ref to branch from. Defaults to the source's current HEAD.
	Base string

	// Instance overrides the derived Docker instance name.
	Instance string
}

// Result describes a created worktree.
type Result struct {
	// Path is the worktree location.
	Path string

	// Branch is the branch checked out in the worktree.
	Branch string

	// SourcePath is the checkout it was created from.
	SourcePath string

	// SourceBranch is the branch the source was on at creation time.
	SourceBranch string

	// Instance is the Docker instance name for the worktree.
	Instance string

	// CreatedAt is when the worktree was created.
	CreatedAt time.Time
}

// Preflight validates a request without changing anything.
//
// It runs before any mutation so that a rejected request leaves no partial
// state behind: no directory, no branch, no registry entry.
func Preflight(ctx context.Context, sourcePath, branch string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("%w: branch name is empty", ErrInvalidBranch)
	}

	dirName := BranchToDirName(branch)
	if dirName == "" {
		return fmt.Errorf("%w: %q does not yield a usable directory name", ErrInvalidBranch, branch)
	}

	xfPath := filepath.Join(sourcePath, "src", "XF.php")
	if _, err := os.Stat(xfPath); err != nil {
		return fmt.Errorf("%w: src/XF.php not found in %s", ErrNotXenForo, sourcePath)
	}

	exists, err := BranchExists(ctx, sourcePath, branch)
	if err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("%w: %s", ErrBranchExists, branch)
	}

	// The branch-to-directory mapping is lossy, so a different branch may
	// already own this directory. Check the path, not just the branch.
	target := filepath.Join(WorktreesDir(sourcePath), dirName)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%w: %s", ErrWorktreeExists, target)
	}

	return nil
}

// Create makes a worktree on a new branch.
//
// It does not configure Docker or install anything; that is the caller's job.
// Pre-flight runs first, so a failure here leaves the source checkout untouched.
func Create(ctx context.Context, opts Options) (*Result, error) {
	source, err := SourceCheckout(ctx, opts.SourcePath)
	if err != nil {
		return nil, err
	}

	if err := Preflight(ctx, source, opts.Branch); err != nil {
		return nil, err
	}

	sourceBranch, err := CurrentBranch(ctx, source)
	if err != nil {
		return nil, err
	}

	target := ResolvePath(source, opts.Branch)

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	args := []string{"worktree", "add", "--quiet", target, "-b", opts.Branch}
	if opts.Base != "" {
		args = append(args, opts.Base)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = source

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		// Leave no empty container directory behind after a failure.
		_ = os.Remove(filepath.Dir(target))

		return nil, fmt.Errorf("failed to create worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	instance := opts.Instance
	if instance == "" {
		instance = xf.GenerateInstanceName(BranchToDirName(opts.Branch))
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
