package worktree

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

// Clean reports whether the worktree holds no work that removal would lose.
func (s WorktreeStatus) Clean() bool {
	return len(s.Modified) == 0 && len(s.UnmergedCommits) == 0
}

// Status inspects a worktree for work that would be lost by removing it.
func Status(ctx context.Context, worktreePath string) (WorktreeStatus, error) {
	var status WorktreeStatus

	// --porcelain includes untracked files, which are easy to forget and just
	// as easy to lose.
	out, err := gitOutput(ctx, worktreePath, "status", "--porcelain")
	if err != nil {
		return status, fmt.Errorf("failed to inspect worktree: %w", err)
	}

	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			status.Modified = append(status.Modified, trimmed)
		}
	}

	// "git remote" with no configured remotes exits 0 with empty output, so a
	// failure here is a real inspection problem, not the "no remotes" case.
	remotes, err := gitOutput(ctx, worktreePath, "remote")
	if err != nil {
		return status, fmt.Errorf("failed to list remotes: %w", err)
	}

	// "Unpushed" is only meaningful when there is somewhere to push to. In a
	// repository with no remotes every commit is unreachable from a remote, so
	// the check would flag every worktree and be worse than useless.
	if strings.TrimSpace(remotes) == "" {
		return status, nil
	}

	// An unborn branch (no commits yet) has no HEAD to inspect, and "git log"
	// on one fails distinctly from a real command failure, so check for that
	// case explicitly rather than treating every "log" error as "no commits".
	if _, err := gitOutput(ctx, worktreePath, "rev-parse", "--verify", "HEAD"); err != nil {
		return status, nil
	}

	// Commits reachable from HEAD but from no remote branch exist only here.
	// HEAD must be named explicitly: "--not --remotes" alone gives git no
	// starting point and silently lists nothing.
	commits, err := gitOutput(ctx, worktreePath, "log", "--oneline", "HEAD", "--not", "--remotes")
	if err != nil {
		return status, fmt.Errorf("failed to inspect commit history: %w", err)
	}

	for _, line := range strings.Split(commits, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			status.UnmergedCommits = append(status.UnmergedCommits, trimmed)
		}
	}

	return status, nil
}

// CheckRemovable reports whether a worktree can be removed without losing work.
//
// It is separate from Remove so callers can run the check before destroying
// anything the removal depends on. Tearing down containers and volumes first
// and only then discovering that the worktree is dirty would refuse the
// removal after the data it was protecting had already been deleted.
func CheckRemovable(ctx context.Context, worktreePath string) error {
	status, err := Status(ctx, worktreePath)
	if err != nil {
		return err
	}

	if len(status.Modified) > 0 {
		return fmt.Errorf("%w:\n  %s", ErrDirtyWorktree, strings.Join(status.Modified, "\n  "))
	}

	if len(status.UnmergedCommits) > 0 {
		return fmt.Errorf("%w:\n  %s", ErrUnmergedCommits, strings.Join(status.UnmergedCommits, "\n  "))
	}

	return nil
}

// Remove deletes a worktree and its branch.
//
// Unless force is set, it refuses when the worktree holds uncommitted changes
// or commits that exist nowhere else, listing what would be lost. Removing
// containers and volumes is the caller's responsibility.
func Remove(ctx context.Context, sourcePath, worktreePath string, force bool) error {
	if !force {
		if err := CheckRemovable(ctx, worktreePath); err != nil {
			return err
		}
	}

	branch, err := CurrentBranch(ctx, worktreePath)
	if err != nil {
		branch = ""
	}

	args := []string{"worktree", "remove", worktreePath}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = sourcePath

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return fmt.Errorf("failed to remove worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	if branch == "" || branch == "HEAD" {
		return nil
	}

	// Deleting the branch is best effort: the worktree is already gone, and a
	// branch that will not delete is not worth failing the whole operation for.
	deleteArgs := []string{"branch", "-D", branch}

	del := exec.CommandContext(ctx, "git", deleteArgs...)
	del.Dir = sourcePath
	_ = del.Run()

	return nil
}
