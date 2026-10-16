package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/worktree"
)

// runGitIn runs git in dir and fails the test if it does not succeed, with the
// same isolated configuration the worktree package's tests use.
func runGitIn(t *testing.T, dir string, args ...string) {
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

// newGitXenForoRepo creates a git repository that looks like a XenForo checkout.
func newGitXenForoRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runGitIn(t, dir, "init", "-q", "-b", "main")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test")

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	runGitIn(t, dir, "add", "-A")
	runGitIn(t, dir, "commit", "-qm", "xenforo")

	return dir
}

// sameDir reports whether two paths name the same directory, resolving
// symlinks the way the production identity check does.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()

	resolve := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Clean(resolved)
		}

		return filepath.Clean(path)
	}

	return resolve(a) == resolve(b)
}

// requireJJ skips the test when Jujutsu is not installed.
func requireJJ(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping the Jujutsu integration test")
	}
}

// isolateJJHome points Jujutsu at throwaway HOME and XDG directories so the
// developer's own configuration cannot affect the result.
func isolateJJHome(t *testing.T) {
	t.Helper()

	home := t.TempDir()

	for _, name := range []string{"config", "data", "cache", "state"} {
		if err := os.MkdirAll(filepath.Join(home, name), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))

	if original, ok := os.LookupEnv("JJ_CONFIG"); ok {
		if err := os.Unsetenv("JJ_CONFIG"); err != nil {
			t.Fatalf("unset JJ_CONFIG: %v", err)
		}

		t.Cleanup(func() { _ = os.Setenv("JJ_CONFIG", original) })
	}
}

// runJJIn runs jj in dir and fails the test if it does not succeed.
func runJJIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "jj", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

// newJJRepo creates a colocated Jujutsu/Git XenForo checkout.
func newJJRepo(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "xenforo")

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	runJJIn(t, dir, "git", "init", "--colocate")
	runJJIn(t, dir, "config", "set", "--user", "user.name", "Test")
	runJJIn(t, dir, "config", "set", "--user", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php\n"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	runJJIn(t, dir, "status")

	return dir
}

// TestWorktreeTargetPrefersTheConventionalCheckout covers the rule that a
// record never overrides a checkout at the conventional location, so removal's
// own branch check still sees the real one.
func TestWorktreeTargetPrefersTheConventionalCheckout(t *testing.T) {
	repo := newGitXenForoRepo(t)
	registry := useTempRegistry(t)

	created, err := worktree.Create(t.Context(), worktree.Options{SourcePath: repo, Branch: "feature"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A competing record for the same branch at another location.
	if err := registry.Add(worktree.Entry{
		SourcePath:   repo,
		WorktreePath: filepath.Join(t.TempDir(), "adopted"),
		Branch:       "feature",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := worktreeTarget(t.Context(), repo, "feature")
	if err != nil {
		t.Fatalf("worktreeTarget: %v", err)
	}

	if !sameDir(t, got, created.Path) {
		t.Errorf("worktreeTarget = %q, want the conventional %q", got, created.Path)
	}
}

// TestWorktreeTargetFallsBackToTheRegistry covers the adopted case: a checkout
// outside the convention is found through its record.
func TestWorktreeTargetFallsBackToTheRegistry(t *testing.T) {
	repo := newGitXenForoRepo(t)
	registry := useTempRegistry(t)

	adopted := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(adopted, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := registry.Add(worktree.Entry{
		SourcePath:   repo,
		SourceBranch: "main",
		WorktreePath: adopted,
		Branch:       "feature",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := worktreeTarget(t.Context(), repo, "feature")
	if err != nil {
		t.Fatalf("worktreeTarget: %v", err)
	}

	if !sameDir(t, got, adopted) {
		t.Errorf("worktreeTarget = %q, want the recorded %q", got, adopted)
	}
}

// TestWorktreeTargetReportsAmbiguousRecords covers two records on one branch:
// choosing one arbitrarily could act on the wrong checkout.
func TestWorktreeTargetReportsAmbiguousRecords(t *testing.T) {
	repo := newGitXenForoRepo(t)
	registry := useTempRegistry(t)

	for _, name := range []string{"one", "two"} {
		if err := registry.Add(worktree.Entry{
			SourcePath:   repo,
			WorktreePath: filepath.Join(t.TempDir(), name),
			Branch:       "feature",
		}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	_, err := worktreeTarget(t.Context(), repo, "feature")
	if err == nil || !strings.Contains(err.Error(), "several registered worktrees") {
		t.Fatalf("worktreeTarget = %v, want an ambiguity error", err)
	}
}

func TestWorktreeTargetReportsAMissingCheckout(t *testing.T) {
	repo := newGitXenForoRepo(t)
	useTempRegistry(t)

	_, err := worktreeTarget(t.Context(), repo, "feature")
	if !errors.Is(err, worktree.ErrWorktreeNotFound) {
		t.Fatalf("worktreeTarget = %v, want ErrWorktreeNotFound", err)
	}
}

// TestWorktreeManagementSourceUsesTheMainCheckoutFromALinkedWorktree covers the
// early return: from a conventional linked worktree, the source is the sibling
// main checkout, so the registry is never consulted.
func TestWorktreeManagementSourceUsesTheMainCheckoutFromALinkedWorktree(t *testing.T) {
	repo := newGitXenForoRepo(t)
	useTempRegistry(t)

	wt := filepath.Join(t.TempDir(), "linked")
	runGitIn(t, repo, "worktree", "add", "-q", wt, "-b", "feature")

	got, err := worktreeManagementSource(t.Context(), wt)
	if err != nil {
		t.Fatalf("worktreeManagementSource: %v", err)
	}

	if !sameDir(t, got, repo) {
		t.Errorf("worktreeManagementSource = %q, want the main checkout %q", got, repo)
	}
}

// TestWorktreeManagementSourceRedirectsAnAdoptedWorkspace covers the reason the
// registry is consulted at all: a Jujutsu workspace at an arbitrary location is
// its own conventional source, so only its record names the repository host
// that list, path and remove consult.
func TestWorktreeManagementSourceRedirectsAnAdoptedWorkspace(t *testing.T) {
	requireJJ(t)
	isolateJJHome(t)

	registry := useTempRegistry(t)

	source := newJJRepo(t)

	workspace := filepath.Join(t.TempDir(), "elsewhere")
	runJJIn(t, source, "workspace", "add", workspace)

	if err := registry.Add(worktree.Entry{
		SourcePath:   source,
		SourceBranch: "default",
		WorktreePath: workspace,
		Branch:       "elsewhere",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := worktreeManagementSource(t.Context(), workspace)
	if err != nil {
		t.Fatalf("worktreeManagementSource: %v", err)
	}

	if !sameDir(t, got, source) {
		t.Errorf("worktreeManagementSource = %q, want the repository host %q", got, source)
	}
}

func TestPathWithin(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		base   string
		target string
		want   bool
	}{
		{"the same path", "/a/b", "/a/b", true},
		{"a child", "/a/b", "/a/b/c", true},
		{"a sibling with a shared prefix", "/a/b", "/a/bc", false},
		{"the parent", "/a/b/c", "/a/b", false},
		{"an unrelated path", "/a/b", "/x/y", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := pathWithin(tc.base, tc.target); got != tc.want {
				t.Errorf("pathWithin(%q, %q) = %v, want %v", tc.base, tc.target, got, tc.want)
			}
		})
	}
}

func TestComparablePathResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()

	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if comparablePath(alias) != comparablePath(real) {
		t.Errorf("comparablePath(%q) = %q, want it to equal comparablePath(%q) = %q",
			alias, comparablePath(alias), real, comparablePath(real))
	}
}
