package worktree

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotARepository indicates a path is not inside a git repository.
var ErrNotARepository = errors.New("not a git repository")

// SourceCheckout returns the main checkout for the repository containing dir.
//
// When dir is inside a linked worktree this returns the *original* checkout,
// not the worktree. That keeps worktrees siblings of the source rather than
// nesting them, so running the command from within a worktree behaves the same
// as running it from the source.
func SourceCheckout(ctx context.Context, dir string) (string, error) {
	// --git-common-dir points at the shared .git directory, which belongs to the
	// main checkout even when called from a linked worktree.
	out, err := gitOutput(ctx, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrNotARepository, dir)
	}

	gitDir := out
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}

	// The checkout is the parent of its .git directory.
	checkout := filepath.Dir(filepath.Clean(gitDir))

	abs, err := filepath.Abs(checkout)
	if err != nil {
		return "", fmt.Errorf("failed to resolve checkout path: %w", err)
	}

	return abs, nil
}

// BranchExists reports whether a local branch of the given name exists.
func BranchExists(ctx context.Context, repoDir, branch string) (bool, error) {
	ref := "refs/heads/" + branch

	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoDir

	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	// show-ref exits 1 when the ref is absent, which is not an error here.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("failed to check branch %q: %w", branch, err)
}

// CurrentBranch returns the branch checked out in repoDir.
func CurrentBranch(ctx context.Context, repoDir string) (string, error) {
	out, err := gitOutput(ctx, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to determine current branch: %w", err)
	}

	return out, nil
}

// gitOutput runs git in dir and returns its trimmed standard output.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}
