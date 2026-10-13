package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitBackend implements backend with Git worktrees.
type gitBackend struct{}

// sourceCheckout returns the main checkout for the repository containing dir.
// If dir is inside the main checkout, the result keeps dir's spelling of the
// path, so symlinks in it are preserved.
func (b gitBackend) sourceCheckout(ctx context.Context, dir string) (string, error) {
	checkouts, err := b.checkouts(ctx, dir)
	if err != nil {
		return "", fmt.Errorf("failed to determine the source checkout: %w", err)
	}

	main, err := mainCheckout(checkouts)
	if err != nil {
		return "", fmt.Errorf("failed to determine the source checkout: %w", err)
	}

	if spelling, ok := mainRootSpelling(dir, main.path); ok {
		return spelling, nil
	}

	abs, err := filepath.Abs(main.path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve checkout path: %w", err)
	}

	return abs, nil
}

// mainRootSpelling returns the first of dir and its ancestors that resolves to
// mainRoot. The boolean reports whether one was found.
func mainRootSpelling(dir, mainRoot string) (string, bool) {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}

	root := resolveSymlinks(mainRoot)

	for spelling := dirAbs; ; spelling = filepath.Dir(spelling) {
		if resolveSymlinks(spelling) == root {
			return spelling, true
		}

		if filepath.Dir(spelling) == spelling {
			return "", false
		}
	}
}

// linkedCheckout describes the worktree containing dir. It returns an error
// for a worktree with a detached HEAD.
func (b gitBackend) linkedCheckout(ctx context.Context, dir string) (checkoutInfo, error) {
	out, err := vcsOutput(ctx, "git", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to determine the checkout root: %w", err)
	}

	root, err := filepath.Abs(out)
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to resolve checkout root: %w", err)
	}

	checkouts, err := b.checkouts(ctx, dir)
	if err != nil {
		return checkoutInfo{}, err
	}

	main, err := mainCheckout(checkouts)
	if err != nil {
		return checkoutInfo{}, err
	}

	want := resolveSymlinks(root)

	for _, c := range checkouts {
		if resolveSymlinks(c.path) != want {
			continue
		}

		if c.name == "" {
			return checkoutInfo{}, fmt.Errorf("%w: %s has a detached HEAD", errNoBranch, root)
		}

		return checkoutInfo{root: root, source: main.path, name: c.name}, nil
	}

	return checkoutInfo{}, fmt.Errorf("%w: %s is not a registered Git worktree", errNotAWorktree, root)
}

// mainCheckout returns the first of checkouts, which Git reports as the main
// checkout. It returns an error if that checkout is not an existing directory.
func mainCheckout(checkouts []checkout) (checkout, error) {
	if len(checkouts) == 0 {
		return checkout{}, errors.New("git reported no worktrees")
	}

	main := checkouts[0]

	info, err := os.Stat(main.path)
	if err != nil {
		return checkout{}, fmt.Errorf("the main checkout %s is unavailable: %w", main.path, err)
	}

	if !info.IsDir() {
		return checkout{}, fmt.Errorf("the main checkout %s is not a directory", main.path)
	}

	return main, nil
}

// nameExists reports whether a local branch of the given name exists. A Git
// exit status indicating an absent reference is not an error.
func (gitBackend) nameExists(ctx context.Context, repoDir, branch string) (bool, error) {
	ref := "refs/heads/" + branch

	cmd := vcsCommand(ctx, "git", repoDir, "show-ref", "--verify", "--quiet", ref)

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
// out. Git before 2.36 rejects -z as a usage error, so the listing then falls
// back to newline-separated fields, which cannot represent a path containing a
// newline.
func (gitBackend) checkouts(ctx context.Context, repoDir string) ([]checkout, error) {
	separator := "\x00"

	out, err := vcsOutput(ctx, "git", repoDir, "worktree", "list", "--porcelain", "-z")
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 129 {
		separator = "\n"
		out, err = vcsOutput(ctx, "git", repoDir, "worktree", "list", "--porcelain")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var result []checkout

	for field := range strings.SplitSeq(out, separator) {
		if path, ok := strings.CutPrefix(field, "worktree "); ok {
			result = append(result, checkout{path: path})

			continue
		}

		if ref, ok := strings.CutPrefix(field, "branch "); ok && len(result) > 0 {
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

	del := vcsCommand(ctx, "git", sourcePath, "branch", deleteFlag, branch)
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
