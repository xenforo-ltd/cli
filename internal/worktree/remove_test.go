package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// createdWorktree makes a worktree and returns the source and worktree paths.
func createdWorktree(t *testing.T, branch string) (string, string) {
	t.Helper()

	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{SourcePath: repo, Branch: branch})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	return repo, result.Path
}

func TestRemoveDeletesACleanWorktree(t *testing.T) {
	repo, wt := createdWorktree(t, "feature")

	if err := Remove(t.Context(), repo, wt, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("worktree directory still exists")
	}

	exists, err := BranchExists(t.Context(), repo, "feature")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}

	if exists {
		t.Error("branch was left behind")
	}
}

// TestRemoveRefusesUncommittedChanges is the guard against losing work.
func TestRemoveRefusesUncommittedChanges(t *testing.T) {
	repo, wt := createdWorktree(t, "feature")

	if err := os.WriteFile(filepath.Join(wt, "new-file.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := Remove(t.Context(), repo, wt, false)
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("expected ErrDirtyWorktree, got %v", err)
	}

	if _, statErr := os.Stat(wt); statErr != nil {
		t.Error("a refused removal must leave the worktree intact")
	}
}

func TestRemoveForceDiscardsChanges(t *testing.T) {
	repo, wt := createdWorktree(t, "feature")

	if err := os.WriteFile(filepath.Join(wt, "new-file.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Remove(t.Context(), repo, wt, true); err != nil {
		t.Fatalf("forced Remove: %v", err)
	}

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("forced removal did not delete the worktree")
	}
}

// TestRemoveRefusesUnpushedCommits guards commits that exist nowhere else.
//
// The repository needs a remote for this to be meaningful: with no remote there
// is nowhere to push, so "unpushed" would describe every commit ever made.
func TestRemoveRefusesUnpushedCommits(t *testing.T) {
	repo, wt := createdWorktree(t, "feature")

	addRemote(t, repo)

	if err := os.WriteFile(filepath.Join(wt, "committed.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runGit(t, wt, "add", "-A")
	runGit(t, wt, "commit", "-qm", "local work")

	err := Remove(t.Context(), repo, wt, false)
	if !errors.Is(err, ErrUnmergedCommits) {
		t.Fatalf("expected ErrUnmergedCommits, got %v", err)
	}
}

func TestRemoveUnknownPath(t *testing.T) {
	repo := newXenForoRepo(t)

	err := Remove(t.Context(), repo, filepath.Join(t.TempDir(), "nope"), false)
	if err == nil {
		t.Fatal("expected an error for an unknown worktree path")
	}
}

func TestStatusReportsCleanliness(t *testing.T) {
	_, wt := createdWorktree(t, "feature")

	status, err := Status(t.Context(), wt)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !status.Clean() {
		t.Errorf("a fresh worktree should be clean, got %+v", status)
	}

	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	status, err = Status(t.Context(), wt)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.Clean() {
		t.Error("an untracked file should make the worktree dirty")
	}
}

// TestStatusReturnsErrorOnInspectionFailure guards against treating a broken
// git invocation as "nothing to lose": Status must surface a real command
// failure rather than silently reporting a clean worktree, since Remove would
// otherwise delete the worktree and force-delete its branch unverified.
func TestStatusReturnsErrorOnInspectionFailure(t *testing.T) {
	notARepo := t.TempDir()

	if _, err := Status(t.Context(), notARepo); err == nil {
		t.Fatal("expected an error when inspecting a path that is not a git repository")
	}
}

// addRemote gives a repository a real remote with the current history, so that
// "not present on any remote" can distinguish new commits from existing ones.
func addRemote(t *testing.T, repo string) {
	t.Helper()

	remote := t.TempDir()

	runGit(t, remote, "init", "-q", "--bare")
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "push", "-q", "origin", "HEAD")
	runGit(t, repo, "fetch", "-q", "origin")
}

// TestStatusIgnoresGeneratedEnvironmentFiles covers the promise that a worktree
// which "create" set up is still removable. The Docker environment is written
// into the worktree itself, so counting those files as uncommitted changes
// would refuse every worktree this tool made and push users to --force, which
// also disables the unpushed-commit check.
func TestStatusIgnoresGeneratedEnvironmentFiles(t *testing.T) {
	_, wt := createdWorktree(t, "feature")

	for _, name := range []string{"compose.yaml", "compose.mysql.yaml", ".env", ".dockerignore"} {
		if err := os.WriteFile(filepath.Join(wt, name), []byte("generated"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	status, err := Status(t.Context(), wt)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !status.Clean() {
		t.Errorf("generated environment files were reported as changes: %v", status.Modified)
	}
}

// TestStatusReportsRealUntrackedWork guards the filtering above from hiding
// anything the user actually created.
func TestStatusReportsRealUntrackedWork(t *testing.T) {
	_, wt := createdWorktree(t, "feature")

	if err := os.WriteFile(filepath.Join(wt, "notes.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	status, err := Status(t.Context(), wt)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.Clean() {
		t.Error("an untracked file the user created was not reported")
	}
}

// TestStatusReportsModifiedTrackedGeneratedFile covers the case where a
// generated file is committed to the repository: once tracked, edits to it are
// the user's work and must not be filtered out.
func TestStatusReportsModifiedTrackedGeneratedFile(t *testing.T) {
	_, wt := createdWorktree(t, "feature")

	compose := filepath.Join(wt, "compose.yaml")
	if err := os.WriteFile(compose, []byte("generated"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runGit(t, wt, "add", "-f", "compose.yaml")
	runGit(t, wt, "commit", "-qm", "track compose")

	if err := os.WriteFile(compose, []byte("edited"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	status, err := Status(t.Context(), wt)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.Clean() {
		t.Error("an edit to a tracked compose.yaml was filtered out")
	}
}

// TestRemoveKeepsUnmergedBranchWithoutRemotes covers the case the pre-flight
// check cannot see: with no remotes configured, Status cannot tell whether a
// commit exists elsewhere, so branch deletion must not be forced.
func TestRemoveKeepsUnmergedBranchWithoutRemotes(t *testing.T) {
	repo, wt := createdWorktree(t, "feature")

	if err := os.WriteFile(filepath.Join(wt, "only-here.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runGit(t, wt, "add", "-A")
	runGit(t, wt, "commit", "-qm", "local only")

	if err := Remove(t.Context(), repo, wt, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// The worktree goes, but the branch holding the only copy of that commit
	// must survive so the work remains recoverable.
	exists, err := BranchExists(t.Context(), repo, "feature")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}

	if !exists {
		t.Error("removed a branch whose commits exist nowhere else")
	}
}
