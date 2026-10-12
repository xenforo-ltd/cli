package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateNumbersBranchesSharingAFinalSegment covers the collision the lossy
// mapping creates: the second branch gets a numbered worktree rather than an
// error.
func TestCreateNumbersBranchesSharingAFinalSegment(t *testing.T) {
	repo := newXenForoRepo(t)

	first, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xfs/slack-unfurl"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	second, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xf/slack-unfurl"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if filepath.Base(first.Path) != "slack-unfurl" || first.Instance != "slack-unfurl" {
		t.Errorf("first worktree = %s (%s), want slack-unfurl", first.Path, first.Instance)
	}

	if filepath.Base(second.Path) != "slack-unfurl-2" || second.Instance != "slack-unfurl-2" {
		t.Errorf("second worktree = %s (%s), want slack-unfurl-2", second.Path, second.Instance)
	}
}

// TestLocateFindsAWorktreeByBranchOrName covers both ways of naming a worktree
// on the command line, including a numbered one that its branch cannot
// predict.
func TestLocateFindsAWorktreeByBranchOrName(t *testing.T) {
	repo := newXenForoRepo(t)

	if _, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xfs/feature"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	second, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xf/feature"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := second.Path

	for _, ref := range []string{"dev/xf/feature", "feature-2"} {
		got, err := Locate(t.Context(), repo, ref)
		if err != nil {
			t.Fatalf("Locate(%q): %v", ref, err)
		}

		if got != want {
			t.Errorf("Locate(%q) = %q, want %q", ref, got, want)
		}
	}
}

// TestLocatePrefersABranchMatch ensures a branch named like another worktree's
// directory resolves to its own worktree.
func TestLocatePrefersABranchMatch(t *testing.T) {
	repo := newXenForoRepo(t)

	if _, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/feature"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	own, err := Create(t.Context(), Options{SourcePath: repo, Branch: "feature"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := Locate(t.Context(), repo, "feature")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	if got != own.Path {
		t.Errorf("Locate(feature) = %q, want the worktree on branch feature, %q", got, own.Path)
	}
}

func TestLocateRejectsUnknownAndSourceCheckouts(t *testing.T) {
	repo := newXenForoRepo(t)

	current, err := CurrentBranch(t.Context(), repo)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	for _, ref := range []string{"absent", current, "..", "."} {
		if _, err := Locate(t.Context(), repo, ref); !errors.Is(err, ErrWorktreeNotFound) {
			t.Errorf("Locate(%q) = %v, want ErrWorktreeNotFound", ref, err)
		}
	}
}

// TestOwnerReportsTheBranchUsingADirectory covers the lookup removal uses to
// confirm a directory is a registered checkout.
func TestOwnerReportsTheBranchUsingADirectory(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xfs/slack-unfurl"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := owner(t.Context(), backendFor(t, repo), repo, result.Path)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}

	if got != "dev/xfs/slack-unfurl" {
		t.Errorf("owner = %q, want the branch that owns the worktree", got)
	}
}

func TestOwnerForUnknownPath(t *testing.T) {
	repo := newXenForoRepo(t)

	got, err := owner(t.Context(), backendFor(t, repo), repo, filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("owner: %v", err)
	}

	if got != "" {
		t.Errorf("owner = %q, want empty for an unknown path", got)
	}
}

// TestLocateSkipsDeletedWorktrees covers a worktree directory removed without
// telling Git, which still lists it as prunable.
func TestLocateSkipsDeletedWorktrees(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/feature"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.RemoveAll(result.Path); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if _, err := Locate(t.Context(), repo, "feature"); !errors.Is(err, ErrWorktreeNotFound) {
		t.Errorf("Locate = %v, want ErrWorktreeNotFound for a deleted worktree", err)
	}
}
