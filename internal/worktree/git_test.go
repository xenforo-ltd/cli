package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTestRepo creates a git repository with one commit and returns its path.
func newTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	run("add", "-A")
	run("commit", "-qm", "initial")

	return dir
}

func TestSourceCheckoutFromRepoRoot(t *testing.T) {
	repo := newTestRepo(t)

	got, err := SourceCheckout(t.Context(), repo)
	if err != nil {
		t.Fatalf("SourceCheckout: %v", err)
	}

	if !samePath(t, got, repo) {
		t.Errorf("SourceCheckout = %q, want %q", got, repo)
	}
}

func TestSourceCheckoutFromSubdirectory(t *testing.T) {
	repo := newTestRepo(t)

	sub := filepath.Join(repo, "src", "nested")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := SourceCheckout(t.Context(), sub)
	if err != nil {
		t.Fatalf("SourceCheckout: %v", err)
	}

	if !samePath(t, got, repo) {
		t.Errorf("SourceCheckout = %q, want the repo root %q", got, repo)
	}
}

// TestSourceCheckoutFromWorktreeReturnsMainCheckout is the important case:
// running the command from inside a worktree must anchor new worktrees to the
// original checkout, not nest them inside the current one.
func TestSourceCheckoutFromWorktreeReturnsMainCheckout(t *testing.T) {
	repo := newTestRepo(t)

	wt := filepath.Join(t.TempDir(), "linked")

	cmd := exec.CommandContext(t.Context(), "git", "worktree", "add", "-q", wt, "-b", "linked-branch")
	cmd.Dir = repo

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}

	got, err := SourceCheckout(t.Context(), wt)
	if err != nil {
		t.Fatalf("SourceCheckout: %v", err)
	}

	if !samePath(t, got, repo) {
		t.Errorf("SourceCheckout from a worktree = %q, want the main checkout %q", got, repo)
	}
}

func TestSourceCheckoutOutsideRepository(t *testing.T) {
	if _, err := SourceCheckout(t.Context(), t.TempDir()); err == nil {
		t.Fatal("expected an error outside a git repository")
	}
}

func TestBranchExists(t *testing.T) {
	repo := newTestRepo(t)

	exists, err := BranchExists(t.Context(), repo, "no-such-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}

	if exists {
		t.Error("reported a non-existent branch as existing")
	}

	cmd := exec.CommandContext(t.Context(), "git", "branch", "real-branch")
	cmd.Dir = repo

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

	exists, err = BranchExists(t.Context(), repo, "real-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}

	if !exists {
		t.Error("did not report an existing branch")
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := newTestRepo(t)

	got, err := CurrentBranch(t.Context(), repo)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	if got != "main" && got != "master" {
		t.Errorf("CurrentBranch = %q, want main or master", got)
	}
}

func samePath(t *testing.T, a, b string) bool {
	t.Helper()

	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}

	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}

	return filepath.Clean(ra) == filepath.Clean(rb)
}
