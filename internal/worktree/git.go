package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/gitenv"
)

// gitBackend implements backend with Git worktrees.
type gitBackend struct{}

// sourceCheckout returns the main checkout for the repository containing dir.
//
// The main checkout is taken from Git's own worktree list rather than from the
// parent of the common Git directory: a repository created with
// --separate-git-dir keeps its .git directory somewhere other than beside the
// checkout, and the list is the only reliable record of where the main
// checkout actually is. The list is ordered with the main checkout first.
//
// Detection already established that dir is in a Git repository, so a failure
// here is a Git failure and is reported as one rather than as "not a
// repository". A bare repository, or a main checkout that is missing or not a
// directory, cannot supply the metadata the rest of xf manages by, so it is
// refused explicitly instead of guessed at.
func (gitBackend) sourceCheckout(ctx context.Context, dir string) (string, error) {
	checkouts, err := gitWorktrees(ctx, dir)
	if err != nil {
		return "", fmt.Errorf("failed to determine the source checkout: %w", err)
	}

	main, err := mainCheckout(checkouts)
	if err != nil {
		return "", fmt.Errorf("failed to determine the source checkout: %w", err)
	}

	abs, err := filepath.Abs(main.root)
	if err != nil {
		return "", fmt.Errorf("failed to resolve checkout path: %w", err)
	}

	// Git reports canonical paths, but the caller may have reached the source
	// through a symlink. When dir is inside the main checkout, walking up from
	// dir keeps the source spelled the way the caller sees it, so derived
	// worktree paths stay where the caller expects them.
	if spelling, ok := mainRootSpelling(dir, main.root); ok {
		return spelling, nil
	}

	return abs, nil
}

// mainRootSpelling returns dir's own spelling of the main checkout root, when
// dir lies inside the main checkout.
//
// It compares canonical paths to decide containment but climbs the caller's
// path, so a source reached through a symlinked directory keeps that spelling.
func mainRootSpelling(dir, mainRoot string) (string, bool) {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}

	rel, err := filepath.Rel(resolveSymlinks(mainRoot), resolveSymlinks(dirAbs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	spelling := dirAbs

	// Climb the caller's path rather than counting components relative to the
	// resolved root: a symlinked component can stand for several canonical
	// levels, so a component count climbs past the root. The caller's spelling
	// of the source is the first ancestor whose canonical path is the main
	// root.
	for {
		if resolveSymlinks(spelling) == resolveSymlinks(mainRoot) {
			return spelling, true
		}

		parent := filepath.Dir(spelling)
		if parent == spelling {
			return "", false
		}

		spelling = parent
	}
}

// linkedCheckout discovers the registered Git worktree containing dir.
//
// The reported root must be one of the worktrees Git itself lists, so a
// directory that merely looks like a checkout is not accepted. The first
// record is the main checkout and supplies the source; the matching record
// supplies the branch. A detached target has no branch to address it by, so it
// is refused with errNoBranch.
func (gitBackend) linkedCheckout(ctx context.Context, dir string) (checkoutInfo, error) {
	out, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to determine the checkout root: %w", err)
	}

	root, err := filepath.Abs(out)
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to resolve checkout root: %w", err)
	}

	checkouts, err := gitWorktrees(ctx, dir)
	if err != nil {
		return checkoutInfo{}, err
	}

	main, err := mainCheckout(checkouts)
	if err != nil {
		return checkoutInfo{}, err
	}

	want := resolveSymlinks(root)

	for _, wt := range checkouts {
		if resolveSymlinks(wt.root) != want {
			continue
		}

		if wt.detached || wt.branch == "" {
			return checkoutInfo{}, fmt.Errorf("%w: %s has a detached HEAD", errNoBranch, root)
		}

		return checkoutInfo{root: root, source: main.root, name: wt.branch}, nil
	}

	return checkoutInfo{}, fmt.Errorf("%w: %s is not a registered Git worktree", errNotAWorktree, root)
}

// mainCheckout returns the main checkout from Git's worktree list.
//
// The first record is documented to be the main checkout. A bare repository
// has no checkout to return, and a main checkout that has gone missing cannot
// supply the source metadata, so both are refused rather than approximated.
func mainCheckout(checkouts []gitWorktree) (gitWorktree, error) {
	main := checkouts[0]

	if main.bare {
		return gitWorktree{}, fmt.Errorf("the repository is bare; a separate main checkout is required")
	}

	info, err := os.Stat(main.root)
	if err != nil {
		return gitWorktree{}, fmt.Errorf("the main checkout %s is unavailable: %w", main.root, err)
	}

	if !info.IsDir() {
		return gitWorktree{}, fmt.Errorf("the main checkout %s is not a directory", main.root)
	}

	return main, nil
}

// gitWorktree is one record from "git worktree list --porcelain -z".
type gitWorktree struct {
	// root is the worktree's absolute path as Git reports it.
	root string

	// branch is the short local branch name, empty for a detached checkout.
	branch string

	// bare reports a bare repository, which has no checkout on disk.
	bare bool

	// detached reports a checkout with no branch checked out.
	detached bool
}

// gitWorktrees lists the repository's worktrees, main checkout first.
func gitWorktrees(ctx context.Context, dir string) ([]gitWorktree, error) {
	out, err := gitOutput(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	checkouts := parseWorktreeList(out)
	if len(checkouts) == 0 {
		return nil, fmt.Errorf("git reported no worktrees for %s", dir)
	}

	return checkouts, nil
}

// parseWorktreeList splits "git worktree list --porcelain -z" output into
// records.
//
// -z terminates every attribute with NUL and each record with an additional
// NUL, so an empty field marks a record boundary. Attributes other than the
// ones xf uses (locked, prunable, HEAD) are ignored without losing the
// boundary, and a path containing spaces or newlines therefore survives
// intact.
func parseWorktreeList(out string) []gitWorktree {
	var (
		checkouts []gitWorktree
		current   *gitWorktree
	)

	for field := range strings.SplitSeq(out, "\x00") {
		if field == "" {
			if current != nil {
				checkouts = append(checkouts, *current)
				current = nil
			}

			continue
		}

		if current == nil {
			current = &gitWorktree{}
		}

		switch {
		case strings.HasPrefix(field, "worktree "):
			current.root = strings.TrimPrefix(field, "worktree ")

		case strings.HasPrefix(field, "branch "):
			current.branch = strings.TrimPrefix(strings.TrimPrefix(field, "branch "), "refs/heads/")

		case field == "bare":
			current.bare = true

		case field == "detached":
			current.detached = true
		}
	}

	// A final record without a trailing separator is still a record.
	if current != nil {
		checkouts = append(checkouts, *current)
	}

	return checkouts
}

// nameExists reports whether a local branch of the given name exists.
func (gitBackend) nameExists(ctx context.Context, repoDir, branch string) (bool, error) {
	ref := "refs/heads/" + branch

	cmd := gitCommand(ctx, repoDir, "show-ref", "--verify", "--quiet", ref)

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

// checkouts lists the repository's worktrees with the branch each has checked
// out.
func (gitBackend) checkouts(ctx context.Context, repoDir string) ([]checkout, error) {
	out, err := gitOutput(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var result []checkout

	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			result = append(result, checkout{path: strings.TrimPrefix(line, "worktree ")})

		case strings.HasPrefix(line, "branch ") && len(result) > 0:
			// Reported as a full ref, e.g. refs/heads/dev/xfs/feature.
			result[len(result)-1].name = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}

	return result, nil
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

	cmd := gitCommand(ctx, source, args...)

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

	cmd := gitCommand(ctx, sourcePath, args...)

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

	del := gitCommand(ctx, sourcePath, "branch", deleteFlag, branch)
	_ = del.Run()

	return nil
}

// gitCommand builds a git command that runs in dir and resolves the repository
// from it.
//
// Repository-location variables are removed from the environment: xf names the
// directory to work in, and a GIT_DIR or GIT_WORK_TREE inherited from a hook or
// a nested Git invocation would override it, making the command act on a
// different checkout.
func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitenv.Environment()

	return cmd
}

// gitOutput runs git in dir and returns its trimmed standard output.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := gitCommand(ctx, dir, args...)

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
