package worktree

import (
	"path/filepath"
	"testing"
)

func TestBranchToDirName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{name: "simple", branch: "feature", want: "feature"},
		{name: "last segment used", branch: "dev/24x/feature", want: "feature"},
		{name: "leading slash trimmed", branch: "/leading", want: "leading"},
		{name: "trailing slash trimmed", branch: "trailing/", want: "trailing"},
		{name: "consecutive slashes collapse", branch: "a//b", want: "b"},
		{name: "spaces become dashes", branch: "my feature", want: "my-feature"},
		{name: "uppercase preserved", branch: "dev/MyAddon/Fix", want: "Fix"},
		{name: "dots preserved", branch: "release/2.4.0", want: "2.4.0"},
		{name: "path traversal neutralised", branch: "../escape", want: "escape"},
		{name: "unsafe characters stripped", branch: "feat:x*y?", want: "feat-x-y"},
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

// TestBranchToDirNameNeverEscapes is the security-relevant property: whatever
// the branch name, the result must be a single path segment that cannot climb
// out of the worktrees directory.
func TestBranchToDirNameNeverEscapes(t *testing.T) {
	t.Parallel()

	for _, branch := range []string{
		"../../etc/passwd",
		"..",
		".",
		"/absolute/path",
		"a/../../b",
		"....//",
	} {
		got := BranchToDirName(branch)

		if got == "" {
			continue // rejected outright, which is also safe
		}

		if filepath.Base(got) != got {
			t.Errorf("BranchToDirName(%q) = %q, which is not a single path segment", branch, got)
		}

		if got == ".." || got == "." {
			t.Errorf("BranchToDirName(%q) = %q, which escapes or self-references", branch, got)
		}
	}
}

func TestWorktreesDir(t *testing.T) {
	t.Parallel()

	got := WorktreesDir("/Users/x/Sites/main")
	want := filepath.Join("/Users/x/Sites", "main.worktrees")

	if got != want {
		t.Errorf("WorktreesDir = %q, want %q", got, want)
	}
}

func TestResolvePath(t *testing.T) {
	t.Parallel()

	got := ResolvePath("/Users/x/Sites/main", "dev/24x/feature")
	want := filepath.Join("/Users/x/Sites", "main.worktrees", "feature")

	if got != want {
		t.Errorf("ResolvePath = %q, want %q", got, want)
	}
}

// TestResolvePathIsDeterministic covers the promise that the path can be
// predicted without consulting any state.
func TestResolvePathIsDeterministic(t *testing.T) {
	t.Parallel()

	a := ResolvePath("/Users/x/Sites/main", "dev/24x/feature")
	b := ResolvePath("/Users/x/Sites/main/", "dev/24x/feature")

	if a != b {
		t.Errorf("a trailing separator changed the result: %q vs %q", a, b)
	}
}
