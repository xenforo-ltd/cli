package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runJJ runs jj in dir and fails the test if it does not succeed.
//
// Every jj call in these tests goes through here. The fixture isolates the
// environment first, so none of them inherit the developer's own
// configuration: a custom template or identity would otherwise decide whether
// the suite passes.
func runJJ(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "jj", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

// requireJJ skips the test when Jujutsu is not installed.
//
// Jujutsu is an optional local dependency: these tests drive the real binary
// rather than a stub, so they are only meaningful when it is present. CI does
// not install it, so the skip is the normal case there.
func requireJJ(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping the Jujutsu integration test")
	}
}

// isolateJJEnvironment points Jujutsu at throwaway HOME and XDG directories so
// the developer's own jj configuration cannot affect the result.
//
// The variables are set on the test process rather than only on the commands
// run here, so the production code under test inherits the same isolated
// environment when it shells out to jj.
func isolateJJEnvironment(t *testing.T) {
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

	// A developer's JJ_CONFIG takes precedence over the temporary HOME, so
	// clear it rather than letting their real configuration leak back in.
	if original, ok := os.LookupEnv("JJ_CONFIG"); ok {
		if err := os.Unsetenv("JJ_CONFIG"); err != nil {
			t.Fatalf("unset JJ_CONFIG: %v", err)
		}

		t.Cleanup(func() { _ = os.Setenv("JJ_CONFIG", original) })
	}
}

// newJJXenForoRepo creates a colocated Jujutsu/Git repository containing a
// XenForo checkout and returns its path.
//
// The repository carries both .jj and .git markers, so creating a workspace
// here also confirms that detection treats a colocated repository as Jujutsu
// rather than Git.
func newJJXenForoRepo(t *testing.T) string {
	t.Helper()

	requireJJ(t)
	isolateJJEnvironment(t)

	dir := filepath.Join(t.TempDir(), "xenforo")

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	runJJ(t, dir, "git", "init", "--colocate")
	runJJ(t, dir, "config", "set", "--user", "user.name", "Test")
	runJJ(t, dir, "config", "set", "--user", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php // fixture\n"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	// Snapshot the checkout so the revision recorded by a test already includes
	// src/XF.php. Without this the first read of @ would amend the working-copy
	// commit behind the recorded revision and the parent comparison would fail.
	runJJ(t, dir, "status")

	return dir
}

// jjCommitID returns the full commit id the revset resolves to in dir.
func jjCommitID(t *testing.T, dir, revset string) string {
	t.Helper()

	return runJJ(t, dir, "log", "--no-graph", "-r", revset, "--template", "commit_id")
}

// TestJujutsuCreateDefaultsToSourceRevision pins the default base of a new
// workspace to the source's current revision.
//
// Jujutsu's own default is the revision's parents, so without the explicit
// --revision=@ the new workspace's parent would be @- and this test would fail.
func TestJujutsuCreateDefaultsToSourceRevision(t *testing.T) {
	source := newJJXenForoRepo(t)

	before := jjCommitID(t, source, "@")

	result, err := Create(t.Context(), Options{
		SourcePath: source,
		Branch:     "feature",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	parent := jjCommitID(t, result.Path, "@-")
	if parent != before {
		t.Errorf("new workspace parent = %s, want the source revision %s", parent, before)
	}

	got, err := os.ReadFile(filepath.Join(result.Path, "src", "XF.php"))
	if err != nil {
		t.Fatalf("read workspace XF.php: %v", err)
	}

	want, err := os.ReadFile(filepath.Join(source, "src", "XF.php"))
	if err != nil {
		t.Fatalf("read source XF.php: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("workspace XF.php = %q, want the current revision's %q", got, want)
	}

	branch, err := CurrentBranch(t.Context(), result.Path)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	if branch != "feature" {
		t.Errorf("CurrentBranch = %q, want feature", branch)
	}
}

// TestJujutsuRemoveRefusesDirtyWorkspaceAndForcesCleanup covers the removal
// path: an unforced removal must protect a changed file and leave the workspace
// registered, and a forced one must both delete the directory and forget the
// workspace.
func TestJujutsuRemoveRefusesDirtyWorkspaceAndForcesCleanup(t *testing.T) {
	source := newJJXenForoRepo(t)
	b := backendFor(t, source)

	result, err := Create(t.Context(), Options{
		SourcePath: source,
		Branch:     "feature",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.WriteFile(filepath.Join(result.Path, "new-file.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write changed file: %v", err)
	}

	if err := Remove(t.Context(), source, result.Path, false); !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("unforced Remove = %v, want ErrDirtyWorktree", err)
	}

	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("a refused removal must leave the workspace intact: %v", err)
	}

	if exists, err := b.nameExists(t.Context(), source, "feature"); err != nil {
		t.Fatalf("nameExists: %v", err)
	} else if !exists {
		t.Error("a refused removal must leave the workspace registered")
	}

	if err := Remove(t.Context(), source, result.Path, true); err != nil {
		t.Fatalf("forced Remove: %v", err)
	}

	if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
		t.Errorf("forced removal left the workspace directory behind: %v", err)
	}

	if exists, err := b.nameExists(t.Context(), source, "feature"); err != nil {
		t.Fatalf("nameExists: %v", err)
	} else if exists {
		t.Error("forced removal left the workspace registered")
	}
}
