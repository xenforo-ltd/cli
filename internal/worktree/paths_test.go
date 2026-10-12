package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/xf"
)

func TestBaseName(t *testing.T) {
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
		{name: "lowercased", branch: "dev/MyAddon/Fix", want: "fix"},
		{name: "dots become dashes", branch: "release/2.4.0", want: "2-4-0"},
		{name: "underscores become dashes", branch: "dev/my_feature", want: "my-feature"},
		{name: "path traversal neutralised", branch: "../escape", want: "escape"},
		{name: "unsafe characters stripped", branch: "feat:x*y?", want: "feat-x-y"},
		{name: "punctuation-only segment skipped", branch: "feature/...", want: "feature"},
		{name: "no usable segment", branch: "..", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := BaseName(tt.branch); got != tt.want {
				t.Errorf("BaseName(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

// TestBaseNameNeverEscapes is the security-relevant property: whatever the
// branch name, the result must be a single path segment that cannot climb out
// of the worktrees directory.
func TestBaseNameNeverEscapes(t *testing.T) {
	t.Parallel()

	for _, branch := range []string{
		"../../etc/passwd",
		"..",
		".",
		"/absolute/path",
		"a/../../b",
		"....//",
	} {
		got := BaseName(branch)

		if got == "" {
			continue // rejected outright, which is also safe
		}

		if filepath.Base(got) != got {
			t.Errorf("BaseName(%q) = %q, which is not a single path segment", branch, got)
		}

		if got == ".." || got == "." {
			t.Errorf("BaseName(%q) = %q, which escapes or self-references", branch, got)
		}
	}
}

// TestBaseNameTruncatesAtAWordBoundary covers long branch names: the name is
// cut back to a whole word rather than ending mid-word.
func TestBaseNameTruncatesAtAWordBoundary(t *testing.T) {
	t.Parallel()

	branch := "dev/" + strings.Repeat("segment-", 10) + "feature"

	got := BaseName(branch)

	if len(got) > xf.MaxInstanceNameLength {
		t.Fatalf("BaseName = %q, %d characters, want at most %d", got, len(got), xf.MaxInstanceNameLength)
	}

	if want := strings.TrimSuffix(strings.Repeat("segment-", 6), "-"); got != want {
		t.Errorf("BaseName = %q, want %q", got, want)
	}
}

func TestTruncateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  string
	}{
		{name: "short-enough", limit: 20, want: "short-enough"},
		{name: "user-state-ladder-redesign", limit: 19, want: "user-state-ladder"},
		{name: "user-state-ladder-redesign", limit: 17, want: "user-state-ladder"},
		{name: "averyveryverylongword", limit: 8, want: "averyver"},
	}

	for _, tt := range tests {
		if got := truncateName(tt.name, tt.limit); got != tt.want {
			t.Errorf("truncateName(%q, %d) = %q, want %q", tt.name, tt.limit, got, tt.want)
		}
	}
}

func TestNumberedName(t *testing.T) {
	t.Parallel()

	if got := numberedName("feature", 1); got != "feature" {
		t.Errorf("first candidate = %q, want the base name unchanged", got)
	}

	if got := numberedName("feature", 2); got != "feature-2" {
		t.Errorf("second candidate = %q, want feature-2", got)
	}

	// The suffix must fit even when the base already uses the whole limit.
	long := strings.Repeat("a", xf.MaxInstanceNameLength)

	got := numberedName(long, 12)
	if len(got) > xf.MaxInstanceNameLength || !strings.HasSuffix(got, "-12") {
		t.Errorf("numberedName(long, 12) = %q, want a suffixed name within the limit", got)
	}
}

func TestChooseNameUsesTheBaseNameWhenFree(t *testing.T) {
	t.Parallel()

	got, err := ChooseName(t.TempDir(), "dev/xfs/user-state-ladder-redesign", nil)
	if err != nil {
		t.Fatalf("ChooseName: %v", err)
	}

	if got != "user-state-ladder-redesign" {
		t.Errorf("ChooseName = %q, want user-state-ladder-redesign", got)
	}
}

// TestChooseNameNumbersPastOccupiedDirectories is the collision the lossy
// mapping creates: dev/24x/feature and dev/xfs/feature both want "feature".
func TestChooseNameNumbersPastOccupiedDirectories(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "main")

	for _, name := range []string{"feature", "feature-2"} {
		if err := os.MkdirAll(filepath.Join(WorktreesDir(source), name), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	got, err := ChooseName(source, "dev/xfs/feature", nil)
	if err != nil {
		t.Fatalf("ChooseName: %v", err)
	}

	if got != "feature-3" {
		t.Errorf("ChooseName = %q, want feature-3", got)
	}
}

// TestChooseNameNumbersPastNamesTakenElsewhere covers state outside the
// worktrees directory, such as a Docker Compose project left by another
// checkout.
func TestChooseNameNumbersPastNamesTakenElsewhere(t *testing.T) {
	t.Parallel()

	got, err := ChooseName(t.TempDir(), "feature", func(name string) (bool, error) {
		return name == "feature", nil
	})
	if err != nil {
		t.Fatalf("ChooseName: %v", err)
	}

	if got != "feature-2" {
		t.Errorf("ChooseName = %q, want feature-2", got)
	}
}

func TestChooseNameReportsCheckFailures(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("docker unreachable")

	_, err := ChooseName(t.TempDir(), "feature", func(string) (bool, error) {
		return false, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("ChooseName error = %v, want the check's error", err)
	}
}

func TestChooseNameRejectsUnusableBranches(t *testing.T) {
	t.Parallel()

	for _, branch := range []string{"", "..", "///", "...."} {
		if _, err := ChooseName(t.TempDir(), branch, nil); !errors.Is(err, ErrInvalidBranch) {
			t.Errorf("ChooseName(%q) = %v, want ErrInvalidBranch", branch, err)
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
