package worktree

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitBackend implements backend with Git worktrees.
type gitBackend struct{}

// sourceCheckout returns the main checkout for the repository containing dir.
//
// --git-common-dir points at the shared .git directory, which belongs to the
// main checkout even when called from a linked worktree, so the checkout is its
// parent. Detection already established that dir is in a Git repository, so a
// failure here is a Git failure and is reported as one rather than as "not a
// repository".
func (gitBackend) sourceCheckout(ctx context.Context, dir string) (string, error) {
	out, err := gitOutput(ctx, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("failed to determine the source checkout: %w", err)
	}

	gitDir := out
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}

	checkout := filepath.Dir(filepath.Clean(gitDir))

	abs, err := filepath.Abs(checkout)
	if err != nil {
		return "", fmt.Errorf("failed to resolve checkout path: %w", err)
	}

	return abs, nil
}

// nameExists reports whether a local branch of the given name exists.
func (gitBackend) nameExists(ctx context.Context, repoDir, branch string) (bool, error) {
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

// currentName returns the branch checked out in repoDir.
func (gitBackend) currentName(ctx context.Context, repoDir string) (string, error) {
	out, err := gitOutput(ctx, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to determine current branch: %w", err)
	}

	return out, nil
}

// owner returns the branch checked out at a worktree path, or an empty string
// when no worktree is registered there.
//
// This makes a collision actionable: the user is told which branch already owns
// the directory, rather than only that something does.
func (gitBackend) owner(ctx context.Context, repoDir, worktreePath string) (string, error) {
	out, err := gitOutput(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %w", err)
	}

	want, err := filepath.Abs(worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", worktreePath, err)
	}

	want = resolveSymlinks(want)

	var current string

	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = resolveSymlinks(strings.TrimPrefix(line, "worktree "))

		case strings.HasPrefix(line, "branch ") && current == want:
			// Reported as a full ref, e.g. refs/heads/dev/xfs/feature.
			return strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/"), nil
		}
	}

	return "", nil
}

// create adds a worktree at target on a new branch.
func (gitBackend) create(ctx context.Context, source, branch, base, target string) error {
	// "--" keeps git from parsing base as an option: without it a base of
	// "--force" is accepted as a flag and the worktree silently branches from
	// HEAD instead of failing.
	args := []string{"worktree", "add", "--quiet", "-b", branch, "--", target}
	if base != "" {
		args = append(args, base)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = source

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return fmt.Errorf("failed to create worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// status inspects a worktree for work that would be lost by removing it.
func (gitBackend) status(ctx context.Context, worktreePath string) (WorktreeStatus, error) {
	var status WorktreeStatus

	// --porcelain includes untracked files, which are easy to forget and just
	// as easy to lose. -z terminates each entry with NUL so paths containing
	// spaces or newlines survive parsing intact.
	out, err := gitOutput(ctx, worktreePath, "status", "--porcelain", "-z")
	if err != nil {
		return status, fmt.Errorf("failed to inspect worktree: %w", err)
	}

	generated, err := generatedPaths()
	if err != nil {
		return status, err
	}

	for _, entry := range parsePorcelain(out) {
		// "worktree create" writes the Docker environment into the worktree
		// itself, so unless the repository happens to ignore every one of those
		// files they arrive here as untracked changes and removal refuses every
		// worktree this tool made. The only escape would be --force, which also
		// disables the unpushed-commit check, so users would learn to always
		// pass it and lose the protection that matters.
		if entry.untracked && generated[entry.path] {
			continue
		}

		status.Modified = append(status.Modified, entry.display)
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

	for line := range strings.SplitSeq(commits, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			status.UnmergedCommits = append(status.UnmergedCommits, trimmed)
		}
	}

	return status, nil
}

// remove deletes a worktree and, best effort, its branch.
func (gitBackend) remove(ctx context.Context, sourcePath, worktreePath string, force bool) error {
	branch, err := gitBackend{}.currentName(ctx, worktreePath)
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
	//
	// Only --force earns "-D". The pre-flight check cannot see unpushed work in
	// a repository with no remotes, so an unforced "-D" there would delete a
	// branch whose commits exist nowhere else. "-d" makes git apply its own
	// merged-into-HEAD-or-upstream test and refuse otherwise, which costs
	// nothing: a branch left behind is recoverable, a deleted one is not.
	deleteFlag := "-d"
	if force {
		deleteFlag = "-D"
	}

	del := exec.CommandContext(ctx, "git", "branch", deleteFlag, branch)
	del.Dir = sourcePath
	_ = del.Run()

	return nil
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

// porcelainEntry is one record from "git status --porcelain -z".
type porcelainEntry struct {
	// path is the repository-relative path the entry concerns.
	path string

	// display is the original porcelain line, kept for error messages.
	display string

	// untracked reports whether git knows nothing about this path.
	untracked bool
}

// parsePorcelain splits NUL-terminated porcelain output into entries.
//
// Renames and copies emit a second NUL-terminated field for the original path.
// That field is consumed here rather than being mistaken for another entry.
func parsePorcelain(out string) []porcelainEntry {
	var entries []porcelainEntry

	records := strings.Split(out, "\x00")

	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}

		// Format is "XY <path>": two status codes, a space, then the path.
		codes := record[:2]
		path := record[3:]

		// R and C carry their source path in the following record.
		if strings.ContainsAny(codes, "RC") {
			i++
		}

		entries = append(entries, porcelainEntry{
			path:      path,
			display:   record,
			untracked: codes == "??",
		})
	}

	return entries
}

// resolveSymlinks resolves a path for comparison, falling back to the cleaned
// path when it cannot be resolved. Temporary directories on macOS are symlinked
// via /var, so comparing unresolved paths gives false mismatches.
func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}

	return filepath.Clean(path)
}
