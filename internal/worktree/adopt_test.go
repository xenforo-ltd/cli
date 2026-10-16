package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// linkedGitWorktree adds a worktree of repo at a fresh path on a new branch and
// returns the worktree path. The worktree is deliberately outside the
// "<source>.worktrees" convention so adoption has to discover it from Git.
func linkedGitWorktree(t *testing.T, repo, branch string) string {
	t.Helper()

	wt := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo, "worktree", "add", "-q", wt, "-b", branch)

	return wt
}

// gitCapture runs git in dir and returns its trimmed output, using the same
// isolated configuration as runGit.
func gitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}

	return strings.TrimSpace(string(out))
}

// TestAdoptRegistersALinkedGitWorktree is the primary case: a worktree Git
// knows about, at a location xf did not choose, becomes usable by the
// management commands.
func TestAdoptRegistersALinkedGitWorktree(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "dev/xfs/feature")

	result, err := Adopt(t.Context(), AdoptOptions{Path: wt})
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if !samePath(t, result.Path, wt) {
		t.Errorf("Path = %q, want the worktree root %q", result.Path, wt)
	}

	if result.Branch != "dev/xfs/feature" {
		t.Errorf("Branch = %q, want dev/xfs/feature", result.Branch)
	}

	if !samePath(t, result.SourcePath, repo) {
		t.Errorf("SourcePath = %q, want the main checkout %q", result.SourcePath, repo)
	}

	if result.SourceBranch != "main" {
		t.Errorf("SourceBranch = %q, want main", result.SourceBranch)
	}

	// The instance is derived from the branch's final segment, as Create does.
	if result.Instance != "feature" {
		t.Errorf("Instance = %q, want feature", result.Instance)
	}

	if result.CreatedAt.IsZero() {
		t.Error("CreatedAt was not set")
	}
}

// TestAdoptAcceptsAPathInsideTheCheckout covers the documented promise that the
// path may name any directory inside the worktree, not only its root.
func TestAdoptAcceptsAPathInsideTheCheckout(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "feature")

	sub := filepath.Join(wt, "src", "nested")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	result, err := Adopt(t.Context(), AdoptOptions{Path: sub})
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if !samePath(t, result.Path, wt) {
		t.Errorf("Path = %q, want the worktree root %q", result.Path, wt)
	}
}

func TestAdoptKeepsAnExplicitInstance(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "feature")

	result, err := Adopt(t.Context(), AdoptOptions{Path: wt, Instance: "custom"})
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if result.Instance != "custom" {
		t.Errorf("Instance = %q, want the explicit custom", result.Instance)
	}
}

// TestAdoptRefusesTheSourceCheckout guards against registering the original
// checkout as one of its own linked checkouts.
func TestAdoptRefusesTheSourceCheckout(t *testing.T) {
	repo := newXenForoRepo(t)

	_, err := Adopt(t.Context(), AdoptOptions{Path: repo})
	if !errors.Is(err, errInvalidPath) {
		t.Fatalf("Adopt(source) = %v, want errInvalidPath", err)
	}
}

func TestAdoptRefusesADetachedGitCheckout(t *testing.T) {
	repo := newXenForoRepo(t)

	wt := filepath.Join(t.TempDir(), "detached")
	runGit(t, repo, "worktree", "add", "-q", "--detach", wt)

	_, err := Adopt(t.Context(), AdoptOptions{Path: wt})
	if !errors.Is(err, errNoBranch) {
		t.Fatalf("Adopt(detached) = %v, want errNoBranch", err)
	}
}

func TestAdoptRefusesACheckoutWithoutXFphp(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "feature")

	if err := os.Remove(filepath.Join(wt, "src", "XF.php")); err != nil {
		t.Fatalf("remove XF.php: %v", err)
	}

	_, err := Adopt(t.Context(), AdoptOptions{Path: wt})
	if !errors.Is(err, ErrNotXenForo) {
		t.Fatalf("Adopt = %v, want ErrNotXenForo", err)
	}
}

func TestAdoptRefusesAPathOutsideARepository(t *testing.T) {
	_, err := Adopt(t.Context(), AdoptOptions{Path: t.TempDir()})
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Adopt = %v, want ErrNotARepository", err)
	}
}

func TestAdoptRefusesAMissingPath(t *testing.T) {
	_, err := Adopt(t.Context(), AdoptOptions{Path: filepath.Join(t.TempDir(), "absent")})
	if !errors.Is(err, errInvalidPath) {
		t.Fatalf("Adopt = %v, want errInvalidPath", err)
	}
}

func TestAdoptRefusesANonDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Adopt(t.Context(), AdoptOptions{Path: file})
	if !errors.Is(err, errInvalidPath) {
		t.Fatalf("Adopt = %v, want errInvalidPath", err)
	}
}

func TestAdoptRefusesAnEmptyPath(t *testing.T) {
	if _, err := Adopt(t.Context(), AdoptOptions{Path: "   "}); !errors.Is(err, errInvalidPath) {
		t.Fatalf("Adopt = %v, want errInvalidPath", err)
	}
}

// TestAdoptChangesNothingInTheCheckout pins the metadata-only promise: the
// working-copy status and HEAD are identical before and after.
func TestAdoptChangesNothingInTheCheckout(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "feature")

	before := gitCapture(t, wt, "status", "--porcelain", "--untracked-files=all")
	head := gitCapture(t, wt, "rev-parse", "HEAD")

	if _, err := Adopt(t.Context(), AdoptOptions{Path: wt}); err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if after := gitCapture(t, wt, "status", "--porcelain", "--untracked-files=all"); after != before {
		t.Errorf("adoption changed the checkout status:\nbefore %q\nafter  %q", before, after)
	}

	if got := gitCapture(t, wt, "rev-parse", "HEAD"); got != head {
		t.Errorf("adoption moved HEAD from %s to %s", head, got)
	}
}

// TestAdoptAcceptsAJujutsuWorkspace covers the Jujutsu backend, including a
// colocated repository, which detection must treat as Jujutsu.
func TestAdoptAcceptsAJujutsuWorkspace(t *testing.T) {
	source := newJJXenForoRepo(t)

	created, err := Create(t.Context(), Options{SourcePath: source, Branch: "feature"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := Adopt(t.Context(), AdoptOptions{Path: created.Path})
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}

	if !samePath(t, result.Path, created.Path) {
		t.Errorf("Path = %q, want %q", result.Path, created.Path)
	}

	if result.Branch != "feature" {
		t.Errorf("Branch = %q, want feature", result.Branch)
	}

	if !samePath(t, result.SourcePath, source) {
		t.Errorf("SourcePath = %q, want the repository host %q", result.SourcePath, source)
	}
}

func TestAdoptRefusesTheJujutsuRepositoryHost(t *testing.T) {
	source := newJJXenForoRepo(t)

	_, err := Adopt(t.Context(), AdoptOptions{Path: source})
	if !errors.Is(err, errInvalidPath) {
		t.Fatalf("Adopt(host) = %v, want errInvalidPath", err)
	}
}

func TestValidateAdoptedSourceAcceptsAStillLinkedCheckout(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "feature")

	if err := ValidateAdoptedSource(t.Context(), wt, repo); err != nil {
		t.Fatalf("ValidateAdoptedSource: %v", err)
	}
}

func TestValidateAdoptedSourceRejectsADifferentSource(t *testing.T) {
	repo := newXenForoRepo(t)
	wt := linkedGitWorktree(t, repo, "feature")
	other := newXenForoRepo(t)

	if err := ValidateAdoptedSource(t.Context(), wt, other); err == nil {
		t.Fatal("expected the checkout to be rejected for an unrelated recorded source")
	}
}

func TestValidateAdoptedSourceRejectsTheSourceItself(t *testing.T) {
	repo := newXenForoRepo(t)

	if err := ValidateAdoptedSource(t.Context(), repo, repo); err == nil {
		t.Fatal("a source checkout is not a linked checkout of itself")
	}
}

// TestRequireXenForoCheckoutRejectsADirectoryMarker covers the marker existing
// with the wrong type: a directory named src/XF.php is not a checkout.
func TestRequireXenForoCheckoutRejectsADirectoryMarker(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src", "XF.php"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := requireXenForoCheckout(dir); !errors.Is(err, ErrNotXenForo) {
		t.Fatalf("requireXenForoCheckout = %v, want ErrNotXenForo", err)
	}
}
