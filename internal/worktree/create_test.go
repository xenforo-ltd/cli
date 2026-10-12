package worktree

import (
	"errors"
	"os"
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

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-qm", "xenforo")

	return dir
}

func TestPreflightAcceptsAValidRequest(t *testing.T) {
	repo := newXenForoRepo(t)

	if err := preflight(t.Context(), backendFor(t, repo), repo, "dev/24x/feature", "feature"); err != nil {
		t.Errorf("Preflight rejected a valid request: %v", err)
	}
}

func TestPreflightRejectsExistingBranch(t *testing.T) {
	repo := newXenForoRepo(t)

	runGit(t, repo, "branch", "taken")

	err := preflight(t.Context(), backendFor(t, repo), repo, "taken", "taken")
	if !errors.Is(err, ErrBranchExists) {
		t.Errorf("expected ErrBranchExists, got %v", err)
	}
}

func TestPreflightRejectsExistingDirectory(t *testing.T) {
	repo := newXenForoRepo(t)

	target := filepath.Join(WorktreesDir(repo), "feature")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	err := preflight(t.Context(), backendFor(t, repo), repo, "dev/24x/feature", "feature")
	if !errors.Is(err, ErrWorktreeExists) {
		t.Errorf("expected ErrWorktreeExists, got %v", err)
	}
}

func TestPreflightRejectsUnsafeNames(t *testing.T) {
	repo := newXenForoRepo(t)

	for _, name := range []string{"", ".", "..", "../escape", "a/b"} {
		if err := preflight(t.Context(), backendFor(t, repo), repo, "feature", name); !errors.Is(err, ErrInvalidBranch) {
			t.Errorf("preflight with name %q = %v, want ErrInvalidBranch", name, err)
		}
	}
}

func TestPreflightRejectsEmptyBranch(t *testing.T) {
	repo := newXenForoRepo(t)

	for _, branch := range []string{"", "   "} {
		if err := preflight(t.Context(), backendFor(t, repo), repo, branch, "feature"); !errors.Is(err, ErrInvalidBranch) {
			t.Errorf("Preflight(%q) = %v, want ErrInvalidBranch", branch, err)
		}
	}
}

func TestPreflightRejectsNonXenForoDirectory(t *testing.T) {
	repo := newTestRepo(t) // a git repo, but no src/XF.php

	err := preflight(t.Context(), backendFor(t, repo), repo, "feature", "feature")
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

	if want := filepath.Join(WorktreesDir(repo), "feature"); result.Path != want {
		t.Errorf("Path = %q, want %q", result.Path, want)
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

// TestCreateNamesTheInstanceAfterTheWorktree covers the single-name promise:
// the directory, the Docker instance and so the hostname all match.
func TestCreateNamesTheInstanceAfterTheWorktree(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{
		SourcePath: repo,
		Branch:     "dev/24x/User-State-Ladder-Redesign",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.Instance != "user-state-ladder-redesign" {
		t.Errorf("Instance = %q, want user-state-ladder-redesign", result.Instance)
	}

	if filepath.Base(result.Path) != result.Instance {
		t.Errorf("directory %q does not match instance %q", filepath.Base(result.Path), result.Instance)
	}
}

func TestCreateKeepsAnExplicitInstance(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{
		SourcePath: repo,
		Branch:     "dev/24x/feature",
		Instance:   "custom",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.Instance != "custom" || filepath.Base(result.Path) != "feature" {
		t.Errorf("got %s (%s), want directory feature with instance custom", result.Path, result.Instance)
	}
}
