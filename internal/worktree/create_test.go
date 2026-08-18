package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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

	cmd := exec.CommandContext(t.Context(), "git", "add", "-A")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	cmd = exec.CommandContext(t.Context(), "git", "commit", "-qm", "xenforo")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

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

	cmd := exec.CommandContext(t.Context(), "git", "branch", "taken")
	cmd.Dir = repo

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

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

	cmd := exec.CommandContext(t.Context(), "git", "branch", "base-branch")
	cmd.Dir = repo

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

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

	cmd := exec.CommandContext(t.Context(), "git", "branch", "taken")
	cmd.Dir = repo

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

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

	if result.Instance == "" {
		t.Error("Create did not derive an instance name")
	}

	const maxInstanceNameLength = 32
	if len(result.Instance) > maxInstanceNameLength {
		t.Errorf("instance name %q exceeds %d characters", result.Instance, maxInstanceNameLength)
	}
}
