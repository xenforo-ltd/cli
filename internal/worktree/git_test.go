package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGit runs git in dir and fails the test if it does not succeed.
//
// Every git call in these tests goes through here so none of them inherit the
// developer's own configuration. commit.gpgsign asking for a key that CI does
// not have, or a custom init.defaultBranch, would otherwise decide whether the
// suite passes.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newTestRepo creates a git repository with one commit and returns its path.
func newTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		runGit(t, dir, args...)
	}

	// The default branch is named explicitly rather than left to git's own
	// default, which varies by version and configuration.
	run("init", "-q", "-b", "main")
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

	runGit(t, repo, "worktree", "add", "-q", wt, "-b", "linked-branch")

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
	b := backendFor(t, repo)

	exists, err := b.nameExists(t.Context(), repo, "no-such-branch")
	if err != nil {
		t.Fatalf("nameExists: %v", err)
	}

	if exists {
		t.Error("reported a non-existent branch as existing")
	}

	runGit(t, repo, "branch", "real-branch")

	exists, err = b.nameExists(t.Context(), repo, "real-branch")
	if err != nil {
		t.Fatalf("nameExists: %v", err)
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
