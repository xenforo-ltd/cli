package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Jujutsu output is parsed with explicit templates rather than the default
// human-readable formatting, so a change to jj's display or the user's color
// and template configuration cannot silently corrupt the parse. "\0" separates
// fields because a path may contain spaces or newlines.
const (
	// stringify() prints a value without jj's display quoting, which would
	// otherwise turn a workspace name beginning with a dash, such as "--foo",
	// into the literal `"--foo"` and stop it matching or being forgotten.
	jjWorkspaceTemplate = `stringify(name) ++ "\0" ++ stringify(root) ++ "\0"`
	jjDiffTemplate      = `status_char ++ "\0" ++ path ++ "\0"`
	jjCommitTemplate    = `commit_id.short() ++ " " ++ description.first_line() ++ "\n"`

	// jjUnmergedRevset selects revisions reachable from the working-copy commit
	// but from no remote bookmark. The working-copy commit itself is excluded:
	// its content is already reported as a file change, so counting it here too
	// would double-report the same work. ::remote_bookmarks() is empty when
	// there are no remote bookmarks, leaving every local revision.
	jjUnmergedRevset = "(::@ ~ ::remote_bookmarks()) ~ @"
)

// jjBackend implements backend with Jujutsu workspaces.
//
// The CLI's branch argument becomes the workspace name. No bookmarks are
// created, moved or deleted: a Jujutsu bookmark does not follow the working
// copy, so managing one here would hand or lose access to work based on state
// the user never asked xf to own.
type jjBackend struct{}

// sourceCheckout returns the source workspace for dir.
//
// A Jujutsu repository has no immutable main workspace: names are mutable and
// there is no equivalent of Git's shared .git directory to point back at. The
// workspace a command runs from is therefore the natural source, except for the
// child workspaces xf created. Those live directly under the source's
// "<source>.worktrees" directory, so a workspace found there is anchored to the
// sibling source instead, which keeps new workspaces beside the original and
// survives the source workspace being renamed.
func (jjBackend) sourceCheckout(ctx context.Context, dir string) (string, error) {
	out, err := jjOutput(ctx, dir, "workspace", "root")
	if err != nil {
		return "", fmt.Errorf("failed to determine the current workspace: %w", err)
	}

	abs, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("failed to resolve checkout path: %w", err)
	}

	parent := filepath.Dir(abs)
	if !strings.HasSuffix(filepath.Base(parent), worktreesSuffix) {
		return abs, nil
	}

	candidate := strings.TrimSuffix(parent, worktreesSuffix)

	// Confirm the candidate is a registered workspace in this repository before
	// treating it as the source: an arbitrary workspace that merely happens to
	// sit under a ".worktrees" directory is still its own source.
	workspaces, err := jjWorkspaces(ctx, abs)
	if err != nil {
		return "", fmt.Errorf("failed to list workspaces: %w", err)
	}

	want := resolveSymlinks(candidate)

	for _, ws := range workspaces {
		if resolveSymlinks(ws.root) == want {
			return candidate, nil
		}
	}

	return abs, nil
}

// nameExists reports whether a workspace of the given name is registered.
func (jjBackend) nameExists(ctx context.Context, repoDir, name string) (bool, error) {
	workspaces, err := jjWorkspaces(ctx, repoDir)
	if err != nil {
		return false, fmt.Errorf("failed to list workspaces: %w", err)
	}

	for _, ws := range workspaces {
		if ws.name == name {
			return true, nil
		}
	}

	return false, nil
}

// currentName returns the name of the workspace checked out at checkoutPath.
func (jjBackend) currentName(ctx context.Context, checkoutPath string) (string, error) {
	root, err := jjOutput(ctx, checkoutPath, "workspace", "root")
	if err != nil {
		return "", fmt.Errorf("failed to determine current workspace: %w", err)
	}

	name, err := jjBackend{}.owner(ctx, checkoutPath, root)
	if err != nil {
		return "", err
	}

	return name, nil
}

// owner returns the workspace whose root is checkoutPath, or an empty string
// when no workspace is registered there.
func (jjBackend) owner(ctx context.Context, repoDir, checkoutPath string) (string, error) {
	workspaces, err := jjWorkspaces(ctx, repoDir)
	if err != nil {
		return "", fmt.Errorf("failed to list workspaces: %w", err)
	}

	want, err := filepath.Abs(checkoutPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", checkoutPath, err)
	}

	want = resolveSymlinks(want)

	for _, ws := range workspaces {
		if resolveSymlinks(ws.root) == want {
			return ws.name, nil
		}
	}

	return "", nil
}

// create adds a workspace named name at target.
//
// --name= and --revision= keep a name or base that begins with a dash from being
// read as an option, the same protection Git's "--" gives its target. Jujutsu's
// default is to base a new workspace on the current commit's parents, so spell
// out @ when no base was supplied to match Create's current-revision default.
func (jjBackend) create(ctx context.Context, source, name, base, target string) error {
	args := []string{"workspace", "add", "--name=" + name}
	if base == "" {
		base = "@"
	}

	args = append(args, "--revision="+base)
	args = append(args, target)

	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = source

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return fmt.Errorf("failed to create workspace: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// status inspects a workspace for work that would be lost by removing it.
func (jjBackend) status(ctx context.Context, worktreePath string) (WorktreeStatus, error) {
	var status WorktreeStatus

	// jj tracks every file in the working copy, so a file Git would call
	// untracked arrives here as an addition. Additions to the generated Docker
	// environment are filtered below, mirroring Git's untracked-file treatment.
	out, err := jjOutput(ctx, worktreePath, "diff", "--template", jjDiffTemplate)
	if err != nil {
		return status, fmt.Errorf("failed to inspect workspace: %w", err)
	}

	generated, err := generatedPaths()
	if err != nil {
		return status, err
	}

	for _, entry := range parseJJDiff(out) {
		if entry.added && generated[entry.path] {
			continue
		}

		status.Modified = append(status.Modified, entry.display)
	}

	// "jj git remote list" prints nothing when no remote is configured, so a
	// failure here is a real inspection problem, not the "no remotes" case.
	remotes, err := jjOutput(ctx, worktreePath, "git", "remote", "list")
	if err != nil {
		return status, fmt.Errorf("failed to list remotes: %w", err)
	}

	// As with Git, "unpushed" is only meaningful when there is somewhere to
	// push to.
	if strings.TrimSpace(remotes) == "" {
		return status, nil
	}

	commits, err := jjOutput(ctx, worktreePath, "log", "--no-graph", "-r", jjUnmergedRevset, "--template", jjCommitTemplate)
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

// remove forgets the workspace and deletes its checkout directory.
//
// Ownership is validated before anything is deleted: the directory is only
// removed when Jujutsu registers it as a workspace, and the source workspace is
// never a valid target. force is unused: the shared pre-removal checks in
// Remove decide whether the workspace may go, and forgetting one never discards
// committed work, so force must not widen what this backend deletes.
//
// The workspace is forgotten before its directory is removed so a cleanup
// failure leaves an ordinary directory rather than live metadata pointing at a
// partially deleted checkout.
func (jjBackend) remove(ctx context.Context, sourcePath, worktreePath string, _ bool) error {
	name, err := jjBackend{}.owner(ctx, sourcePath, worktreePath)
	if err != nil {
		return err
	}

	if name == "" {
		return fmt.Errorf("no Jujutsu workspace is registered at %s", worktreePath)
	}

	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to resolve source path: %w", err)
	}

	targetAbs, err := filepath.Abs(worktreePath)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	if resolveSymlinks(sourceAbs) == resolveSymlinks(targetAbs) {
		return fmt.Errorf("refusing to remove the source workspace %s", worktreePath)
	}

	// "--" makes a workspace name beginning with a dash a value rather than an
	// option, matching the protection create gets from "--name=".
	cmd := exec.CommandContext(ctx, "jj", "workspace", "forget", "--", name)
	cmd.Dir = sourcePath

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return fmt.Errorf("failed to forget workspace: %w: %s", err, strings.TrimSpace(string(out)))
	}

	if err := os.RemoveAll(worktreePath); err != nil {
		return fmt.Errorf("failed to remove workspace directory: %w", err)
	}

	return nil
}

// jjWorkspace is a workspace name and its recorded root directory.
type jjWorkspace struct {
	name string
	root string
}

// jjWorkspaces lists the repository's workspaces with their roots.
func jjWorkspaces(ctx context.Context, dir string) ([]jjWorkspace, error) {
	out, err := jjOutput(ctx, dir, "workspace", "list", "--template", jjWorkspaceTemplate)
	if err != nil {
		return nil, err
	}

	fields := strings.Split(out, "\x00")

	workspaces := make([]jjWorkspace, 0, len(fields)/2)

	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "" {
			continue
		}

		workspaces = append(workspaces, jjWorkspace{name: fields[i], root: fields[i+1]})
	}

	return workspaces, nil
}

// jjDiffEntry is one changed file from "jj diff".
type jjDiffEntry struct {
	// path is the workspace-relative path the entry concerns.
	path string

	// display is the status code and path, kept for error messages.
	display string

	// added reports whether the file is new to the working-copy commit.
	added bool
}

// parseJJDiff splits NUL-terminated status/path output into entries.
func parseJJDiff(out string) []jjDiffEntry {
	fields := strings.Split(out, "\x00")

	var entries []jjDiffEntry

	for i := 0; i+1 < len(fields); i += 2 {
		status := fields[i]
		if status == "" {
			continue
		}

		path := fields[i+1]

		entries = append(entries, jjDiffEntry{
			path:    path,
			display: status + " " + path,
			added:   status == "A",
		})
	}

	return entries
}

// jjOutput runs jj in dir and returns its trimmed standard output.
func jjOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", args...)
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
