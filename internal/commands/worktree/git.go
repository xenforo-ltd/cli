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
	out, err := vcsOutput(ctx, "git", dir, "rev-parse", "--git-common-dir")
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

// nameExists reports whether a local branch of the given name exists. A Git
// exit status indicating an absent reference is not an error.
func (gitBackend) nameExists(ctx context.Context, repoDir, branch string) (bool, error) {
	ref := "refs/heads/" + branch

	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoDir

	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("failed to check branch %q: %w", branch, err)
}

// currentName returns the branch checked out in repoDir.
func (gitBackend) currentName(ctx context.Context, repoDir string) (string, error) {
	out, err := vcsOutput(ctx, "git", repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to determine current branch: %w", err)
	}

	return out, nil
}

// checkouts lists the repository's worktrees with the branch each has checked
// out.
func (gitBackend) checkouts(ctx context.Context, repoDir string) ([]checkout, error) {
	out, err := vcsOutput(ctx, "git", repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var result []checkout

	for line := range strings.SplitSeq(out, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			result = append(result, checkout{path: path})

			continue
		}

		if ref, ok := strings.CutPrefix(line, "branch "); ok && len(result) > 0 {
			result[len(result)-1].name = strings.TrimPrefix(ref, "refs/heads/")
		}
	}

	return result, nil
}

// create adds a worktree at target on a new branch. A separator prevents a
// base revision beginning with a dash from being interpreted as an option.
func (gitBackend) create(ctx context.Context, source, branch, base, target string) error {
	args := []string{"worktree", "add", "--quiet", "-b", branch, "--", target}
	if base != "" {
		args = append(args, base)
	}

	return vcsRun(ctx, "git", source, "create worktree", args...)
}

// status inspects a worktree for work that would be lost by removing it.
// Generated untracked Docker files are excluded. Unpushed commits are checked
// only when a remote and HEAD exist. A failure to list remotes is reported.
func (gitBackend) status(ctx context.Context, worktreePath string) (worktreeStatus, error) {
	var status worktreeStatus

	out, err := vcsOutput(ctx, "git", worktreePath, "status", "--porcelain", "-z")
	if err != nil {
		return status, fmt.Errorf("failed to inspect worktree: %w", err)
	}

	generated, err := generatedPaths()
	if err != nil {
		return status, err
	}

	for _, entry := range parsePorcelain(out) {
		if entry.untracked && generated[entry.path] {
			continue
		}

		status.modified = append(status.modified, entry.display)
	}

	remotes, err := vcsOutput(ctx, "git", worktreePath, "remote")
	if err != nil {
		return status, fmt.Errorf("failed to list remotes: %w", err)
	}

	if strings.TrimSpace(remotes) == "" {
		return status, nil
	}

	if _, err := vcsOutput(ctx, "git", worktreePath, "rev-parse", "--verify", "HEAD"); err != nil {
		return status, nil
	}

	commits, err := vcsOutput(ctx, "git", worktreePath, "log", "--oneline", "HEAD", "--not", "--remotes")
	if err != nil {
		return status, fmt.Errorf("failed to inspect commit history: %w", err)
	}

	status.unmergedCommits = nonEmptyLines(commits)

	return status, nil
}

// remove deletes a worktree and, best effort, its branch. It uses Git's safe
// branch deletion unless forced, protecting unmerged work when no remote exists.
func (gitBackend) remove(ctx context.Context, sourcePath, worktreePath string, force bool) error {
	branch, err := gitBackend{}.currentName(ctx, worktreePath)
	if err != nil {
		branch = ""
	}

	args := []string{"worktree", "remove", worktreePath}
	if force {
		args = append(args, "--force")
	}

	if err := vcsRun(ctx, "git", sourcePath, "remove worktree", args...); err != nil {
		return err
	}

	if branch == "" || branch == "HEAD" {
		return nil
	}

	deleteFlag := "-d"
	if force {
		deleteFlag = "-D"
	}

	del := exec.CommandContext(ctx, "git", "branch", deleteFlag, branch)
	del.Dir = sourcePath
	_ = del.Run()

	return nil
}

// porcelainEntry records a repository-relative path, its original status line,
// and whether the path is untracked.
type porcelainEntry struct {
	path string

	display string

	untracked bool
}

// parsePorcelain splits NUL-terminated porcelain output into entries.
//
// Renames and copies emit a second NUL-terminated field for the original path.
// That field is consumed here rather than being mistaken for another entry.
// Each record starts with two status codes and a space followed by its path.
func parsePorcelain(out string) []porcelainEntry {
	var entries []porcelainEntry

	records := strings.Split(out, "\x00")

	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}

		codes := record[:2]
		path := record[3:]

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
