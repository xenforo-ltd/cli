package worktree

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchToDirNameUsesLastSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{name: "conventional branch", branch: "dev/xfs/slack-unfurl", want: "slack-unfurl"},
		{name: "two segments", branch: "dev/feature", want: "feature"},
		{name: "single segment", branch: "feature", want: "feature"},
		{name: "trailing slash ignored", branch: "dev/feature/", want: "feature"},
		{name: "dots preserved", branch: "release/2.4.0", want: "2.4.0"},
		{name: "spaces become dashes", branch: "dev/my feature", want: "my-feature"},
		{name: "unsafe characters stripped", branch: "dev/feat:x*y", want: "feat-x-y"},
		{name: "traversal neutralised", branch: "../escape", want: "escape"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := BranchToDirName(tt.branch); got != tt.want {
				t.Errorf("BranchToDirName(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

// TestBranchToDirNameCollides documents the trade-off that last-segment naming
// accepts: different branches can want the same directory. Preflight rejects
// the second one rather than silently renaming it.
func TestBranchToDirNameCollides(t *testing.T) {
	t.Parallel()

	a := BranchToDirName("dev/xfs/slack-unfurl")
	b := BranchToDirName("dev/xf/slack-unfurl")

	if a != b {
		t.Fatalf("expected these to collide, got %q and %q", a, b)
	}
}

// TestPreflightRejectsCollisionWithExistingBranch is the safeguard: a second
// branch wanting an occupied directory must be refused, and the error must name
// the branch already using it so the fix is obvious.
func TestPreflightRejectsCollisionWithExistingBranch(t *testing.T) {
	repo := newXenForoRepo(t)

	if _, err := Create(t.Context(), Options{
		SourcePath: repo,
		Branch:     "dev/xfs/slack-unfurl",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := Preflight(t.Context(), repo, "dev/xf/slack-unfurl")
	if err == nil {
		t.Fatal("expected a colliding branch to be rejected")
	}

	msg := err.Error()

	if !strings.Contains(msg, "slack-unfurl") {
		t.Errorf("error %q does not identify the directory in conflict", msg)
	}

	if !strings.Contains(msg, "dev/xfs/slack-unfurl") {
		t.Errorf("error %q does not name the branch already using it", msg)
	}
}

// TestPreflightAllowsDistinctLastSegments confirms the common case still works.
func TestPreflightAllowsDistinctLastSegments(t *testing.T) {
	repo := newXenForoRepo(t)

	if _, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xfs/one"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Preflight(t.Context(), repo, "dev/xfs/two"); err != nil {
		t.Errorf("distinct feature names must not conflict: %v", err)
	}
}

// TestWorktreeOwnerReportsTheBranchUsingADirectory covers the lookup that makes
// the rejection message actionable.
func TestWorktreeOwnerReportsTheBranchUsingADirectory(t *testing.T) {
	repo := newXenForoRepo(t)

	result, err := Create(t.Context(), Options{SourcePath: repo, Branch: "dev/xfs/slack-unfurl"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	owner, err := worktreeOwner(t.Context(), repo, result.Path)
	if err != nil {
		t.Fatalf("worktreeOwner: %v", err)
	}

	if owner != "dev/xfs/slack-unfurl" {
		t.Errorf("owner = %q, want the branch that owns the worktree", owner)
	}
}

func TestWorktreeOwnerForUnknownPath(t *testing.T) {
	repo := newXenForoRepo(t)

	owner, err := worktreeOwner(t.Context(), repo, filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("worktreeOwner: %v", err)
	}

	if owner != "" {
		t.Errorf("owner = %q, want empty for an unknown path", owner)
	}
}
