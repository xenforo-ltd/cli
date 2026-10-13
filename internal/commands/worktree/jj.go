package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// jjWorkspaceTemplate emits unquoted workspace names and roots separated by NUL.
	jjWorkspaceTemplate = `stringify(name) ++ "\0" ++ stringify(root) ++ "\0"`
	// jjDiffTemplate emits status and path fields separated by NUL.
	jjDiffTemplate = `status_char ++ "\0" ++ path ++ "\0"`
	// jjCommitTemplate emits a short commit ID and first description line.
	jjCommitTemplate = `commit_id.short() ++ " " ++ description.first_line() ++ "\n"`

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
// survives the source workspace being renamed. The candidate must be a
// registered workspace, not merely a similarly named directory.
func (jjBackend) sourceCheckout(ctx context.Context, dir string) (string, error) {
	out, err := jjReadOutput(ctx, dir, "workspace", "root")
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

	workspaces, err := jjWorkspaces(ctx, abs)
	if err != nil {
		return "", fmt.Errorf("failed to list workspaces: %w", err)
	}

	if _, ok := jjWorkspaceAt(workspaces, candidate); ok {
		return candidate, nil
	}

	return abs, nil
}

// linkedCheckout describes the workspace containing dir. Its source is the
// workspace that holds the repository, unless sourceCheckout would anchor it
// to a sibling source.
func (jjBackend) linkedCheckout(ctx context.Context, dir string) (checkoutInfo, error) {
	out, err := jjReadOutput(ctx, dir, "workspace", "root")
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to determine the workspace root: %w", err)
	}

	root, err := filepath.Abs(out)
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to resolve workspace root: %w", err)
	}

	workspaces, err := jjWorkspaces(ctx, dir)
	if err != nil {
		return checkoutInfo{}, fmt.Errorf("failed to list workspaces: %w", err)
	}

	current, ok := jjWorkspaceAt(workspaces, root)
	if !ok {
		return checkoutInfo{}, fmt.Errorf("%w: no Jujutsu workspace is registered at %s", errNotAWorktree, root)
	}

	host, err := jjRepositoryHost(root, workspaces)
	if err != nil {
		return checkoutInfo{}, err
	}

	if resolveSymlinks(host) == resolveSymlinks(root) {
		return checkoutInfo{root: root, source: root, name: current.name}, nil
	}

	source := host

	if parent := filepath.Dir(root); strings.HasSuffix(filepath.Base(parent), worktreesSuffix) {
		if sibling, ok := jjWorkspaceAt(workspaces, strings.TrimSuffix(parent, worktreesSuffix)); ok {
			source = sibling.root
		}
	}

	return checkoutInfo{root: root, source: source, name: current.name}, nil
}

// jjRepositoryHost returns the root of the workspace whose .jj directory holds
// the repository of the workspace at root.
func jjRepositoryHost(root string, workspaces []jjWorkspace) (string, error) {
	repoDir, err := jjRepoDir(root)
	if err != nil {
		return "", err
	}

	if filepath.Base(repoDir) != "repo" || filepath.Base(filepath.Dir(repoDir)) != ".jj" {
		return "", fmt.Errorf("%s does not use the standard .jj/repo layout", root)
	}

	host := filepath.Dir(filepath.Dir(repoDir))

	ws, ok := jjWorkspaceAt(workspaces, host)
	if !ok {
		return "", fmt.Errorf("%w: the repository host %s is not a registered workspace", errNotAWorktree, host)
	}

	return ws.root, nil
}

// jjRepoDir returns the repository directory of the workspace at root, with
// symlinks resolved. In a secondary workspace, .jj/repo is a file that holds
// the path.
func jjRepoDir(root string) (string, error) {
	path := filepath.Join(root, ".jj", "repo")

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to inspect %s: %w", path, err)
	}

	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read %s: %w", path, err)
		}

		path = filepath.FromSlash(string(data))

		if !filepath.IsAbs(path) {
			path = filepath.Join(root, ".jj", path)
		}
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repository path %s: %w", path, err)
	}

	info, err = os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to inspect repository path %s: %w", resolved, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("repository path %s is not a directory", resolved)
	}

	return resolved, nil
}

// nameExists reports whether a workspace of the given name is registered.
func (jjBackend) nameExists(ctx context.Context, repoDir, name string) (bool, error) {
	workspaces, err := jjWorkspaces(ctx, repoDir)
	if err != nil {
		return false, fmt.Errorf("failed to list workspaces: %w", err)
	}

	return slices.ContainsFunc(workspaces, func(ws jjWorkspace) bool { return ws.name == name }), nil
}

// currentName returns the name of the workspace checked out at checkoutPath.
func (jjBackend) currentName(ctx context.Context, checkoutPath string) (string, error) {
	root, err := jjReadOutput(ctx, checkoutPath, "workspace", "root")
	if err != nil {
		return "", fmt.Errorf("failed to determine current workspace: %w", err)
	}

	return owner(ctx, jjBackend{}, checkoutPath, root)
}

// checkouts lists the repository's workspaces with their roots.
func (jjBackend) checkouts(ctx context.Context, repoDir string) ([]checkout, error) {
	workspaces, err := jjWorkspaces(ctx, repoDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	result := make([]checkout, 0, len(workspaces))

	for _, ws := range workspaces {
		result = append(result, checkout{path: ws.root, name: ws.name})
	}

	return result, nil
}

// create adds a workspace named name at target.
//
// --name= and --revision= keep a name or base that begins with a dash from being
// read as an option, the same protection Git's "--" gives its target. Jujutsu's
// default is to base a new workspace on the current commit's parents, so spell
// out @ when no base was supplied to match create's current-revision default.
func (jjBackend) create(ctx context.Context, source, name, base, target string) error {
	args := []string{"workspace", "add", "--name=" + name}
	if base == "" {
		base = "@"
	}

	args = append(args, "--revision="+base)
	args = append(args, target)

	return vcsRun(ctx, "jj", source, "create workspace", args...)
}

// status inspects a workspace for work that would be lost by removing it.
// Generated Docker additions are excluded. Unpushed commits are checked only
// when a remote exists; a failure to list remotes is reported.
func (jjBackend) status(ctx context.Context, worktreePath string) (worktreeStatus, error) {
	var status worktreeStatus

	out, err := vcsOutput(ctx, "jj", worktreePath, "diff", "--template", jjDiffTemplate)
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

		status.modified = append(status.modified, entry.display)
	}

	remotes, err := vcsOutput(ctx, "jj", worktreePath, "git", "remote", "list")
	if err != nil {
		return status, fmt.Errorf("failed to list remotes: %w", err)
	}

	if strings.TrimSpace(remotes) == "" {
		return status, nil
	}

	commits, err := vcsOutput(ctx, "jj", worktreePath, "log", "--no-graph", "-r", jjUnmergedRevset, "--template", jjCommitTemplate)
	if err != nil {
		return status, fmt.Errorf("failed to inspect commit history: %w", err)
	}

	status.unmergedCommits = nonEmptyLines(commits)

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
// A separator protects workspace names that start with a dash.
func (jjBackend) remove(ctx context.Context, sourcePath, worktreePath string, _ bool) error {
	name, err := owner(ctx, jjBackend{}, sourcePath, worktreePath)
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

	if err := vcsRun(ctx, "jj", sourcePath, "forget workspace", "workspace", "forget", "--", name); err != nil {
		return err
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

// jjWorkspaceAt returns the workspace whose root resolves to the same directory
// as path. The boolean reports whether one was found.
func jjWorkspaceAt(workspaces []jjWorkspace, path string) (jjWorkspace, bool) {
	want := resolveSymlinks(path)

	for _, ws := range workspaces {
		if resolveSymlinks(ws.root) == want {
			return ws, true
		}
	}

	return jjWorkspace{}, false
}

// jjWorkspaces lists the repository's workspaces with their roots.
func jjWorkspaces(ctx context.Context, dir string) ([]jjWorkspace, error) {
	out, err := jjReadOutput(ctx, dir, "workspace", "list", "--template", jjWorkspaceTemplate)
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

// jjDiffEntry records a workspace-relative path, its status for display, and
// whether the file is new to the working-copy commit.
type jjDiffEntry struct {
	path string

	display string

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

// jjReadOnlyArgs are the global options for jj commands that only read the
// repository. They skip the working-copy snapshot and disable the pager and
// color.
var jjReadOnlyArgs = []string{"--ignore-working-copy", "--at-operation=@", "--no-pager", "--color=never"}

// jjReadOutput is like vcsOutput for jj, with jjReadOnlyArgs before args.
func jjReadOutput(ctx context.Context, dir string, args ...string) (string, error) {
	return vcsOutput(ctx, "jj", dir, slices.Concat(jjReadOnlyArgs, args)...)
}
