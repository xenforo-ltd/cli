package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/xf"
)

// newXenForoRepo creates a git repo that looks like a XenForo checkout.
func newXenForoRepo(t *testing.T) string {
	t.Helper()

	dir := newTestRepo(t)

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-qm", "xenforo")

	return dir
}

func TestPreflightAcceptsAValidRequest(t *testing.T) {
	repo := newXenForoRepo(t)

	if err := Preflight(t.Context(), repo, "dev/24x/feature"); err != nil {
		t.Errorf("Preflight rejected a valid request: %v", err)
	}
}

func TestPreflightRejectsExistingBranch(t *testing.T) {
	repo := newXenForoRepo(t)

	runGit(t, repo, "branch", "taken")

	err := Preflight(t.Context(), repo, "taken")
	if !errors.Is(err, ErrBranchExists) {
		t.Errorf("expected ErrBranchExists, got %v", err)
	}
}

func TestPreflightRejectsExistingDirectory(t *testing.T) {
	repo := newXenForoRepo(t)

	target := ResolvePath(repo, "dev/24x/feature")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	err := Preflight(t.Context(), repo, "dev/24x/feature")
	if !errors.Is(err, ErrWorktreeExists) {
		t.Errorf("expected ErrWorktreeExists, got %v", err)
	}
}

// TestPreflightRejectsCollidingName covers the lossy branch-to-directory
// mapping: only the last segment names the directory, so branches that differ
// earlier can still want the same one.
func TestPreflightRejectsCollidingName(t *testing.T) {
	repo := newXenForoRepo(t)

	// Both of these reduce to "feature".
	target := ResolvePath(repo, "dev/24x/feature")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	err := Preflight(t.Context(), repo, "dev/xfs/feature")
	if !errors.Is(err, ErrWorktreeExists) {
		t.Errorf("expected a collision to be caught, got %v", err)
	}
}

func TestPreflightRejectsEmptyBranch(t *testing.T) {
	repo := newXenForoRepo(t)

	for _, branch := range []string{"", "   ", "..", "///"} {
		if err := Preflight(t.Context(), repo, branch); !errors.Is(err, ErrInvalidBranch) {
			t.Errorf("Preflight(%q) = %v, want ErrInvalidBranch", branch, err)
		}
	}
}

func TestPreflightRejectsNonXenForoDirectory(t *testing.T) {
	repo := newTestRepo(t) // a git repo, but no src/XF.php

	err := Preflight(t.Context(), repo, "feature")
	if !errors.Is(err, ErrNotXenForo) {
		t.Errorf("expected ErrNotXenForo, got %v", err)
	}
}

func TestCreateMakesAWorktreeOnANewBranch(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{
		SourcePath: repo,
		Branch:     "dev/24x/feature",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.Path != ResolvePath(repo, "dev/24x/feature") {
		t.Errorf("Path = %q, want the resolved path", result.Path)
	}

	if _, err := os.Stat(filepath.Join(result.Path, "src", "XF.php")); err != nil {
		t.Errorf("worktree does not contain the checkout: %v", err)
	}

	branch, err := CurrentBranch(t.Context(), result.Path)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	if branch != "dev/24x/feature" {
		t.Errorf("worktree is on %q, want dev/24x/feature", branch)
	}
}

func TestCreateFromExplicitBase(t *testing.T) {
	repo := newXenForoRepo(t)

	runGit(t, repo, "branch", "base-branch")

	result, err := Create(t.Context(), Options{
		SourcePath: repo,
		Branch:     "from-base",
		Base:       "base-branch",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.Branch != "from-base" {
		t.Errorf("Branch = %q, want from-base", result.Branch)
	}
}

// TestCreateLeavesNoPartialStateOnFailure covers the promise that a failed
// pre-flight does not create anything.
func TestCreateLeavesNoPartialStateOnFailure(t *testing.T) {
	repo := newXenForoRepo(t)

	runGit(t, repo, "branch", "taken")

	if _, err := Create(t.Context(), Options{SourcePath: repo, Branch: "taken"}); err == nil {
		t.Fatal("expected Create to fail")
	}

	if _, err := os.Stat(WorktreesDir(repo)); !os.IsNotExist(err) {
		t.Error("a failed Create left a worktrees directory behind")
	}
}

func TestCreateDerivesInstanceName(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{
		SourcePath: repo,
		Branch:     "dev/24x/feature",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Create must pass the helper-derived value through unchanged.
	if want := InstanceName(result.SourcePath, result.Branch); result.Instance != want {
		t.Errorf("Create derived instance %q, want %q", result.Instance, want)
	}
}

// TestInstanceNameCanonicalizesSourcePath ensures aliases for the same checkout
// do not produce separate Docker environments.
func TestInstanceNameCanonicalizesSourcePath(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	alias := filepath.Join(t.TempDir(), "source")
	if err := os.Symlink(repo, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	canonical := InstanceName(repo, "dev/24x/feature")
	linked := InstanceName(alias, "dev/24x/feature")

	if canonical != linked {
		t.Errorf("canonical path derived %q, symlink derived %q", canonical, linked)
	}
}

// TestInstanceNameSeparatesBranchesSharingAFinalSegment is the collision the
// digest exists to prevent: the directory mapping is lossy, so without it these
// two branches would share a Compose project and stopping one would affect the
// other.
func TestInstanceNameSeparatesBranchesSharingAFinalSegment(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	a := InstanceName(repo, "dev/24x/feature")
	b := InstanceName(repo, "dev/xfs/feature")

	if a == b {
		t.Errorf("branches sharing a final segment derived the same instance name %q", a)
	}
}

// TestInstanceNameSeparatesCheckouts covers the second half of the digest: the
// same branch name in two checkouts must not adopt each other's environment.
func TestInstanceNameSeparatesCheckouts(t *testing.T) {
	t.Parallel()

	const branch = "dev/24x/feature"

	a := InstanceName(t.TempDir(), branch)
	b := InstanceName(t.TempDir(), branch)

	if a == b {
		t.Errorf("the same branch in different checkouts derived the same instance name %q", a)
	}
}

// TestInstanceNameStaysWithinTheLengthLimit covers the readable prefix being
// shortened as the branch grows. The digest must survive intact; only the
// prefix gives way, because truncating the digest would discard the collision
// resistance it provides.
func TestInstanceNameStaysWithinTheLengthLimit(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a-very-long-segment-", 8) + "feature"

	name := InstanceName(t.TempDir(), long)

	if len(name) > xf.MaxInstanceNameLength {
		t.Errorf("instance name %q is %d characters, want at most %d", name, len(name), xf.MaxInstanceNameLength)
	}
}
