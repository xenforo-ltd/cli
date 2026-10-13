package worktree_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/commands/worktree"
)

func vcs(t *testing.T, dir, tool string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), tool, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", tool, strings.Join(args, " "), err, out)
	}
}

func gitFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	gitConfig := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(gitConfig, []byte("[user]\n\tname = Test User\n\temail = test@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if err := os.WriteFile(filepath.Join(repo, "src", "XF.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcs(t, repo, "git", "init", "--quiet", "--initial-branch=main")
	vcs(t, repo, "git", "add", ".")
	vcs(t, repo, "git", "commit", "--quiet", "-m", "initial")
	t.Chdir(repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func adopt(t *testing.T, path string, opts worktree.AdoptOptions) createdWorktree {
	t.Helper()
	opts.JSON = true
	out, err := capture(t, func() error { return worktree.AdoptWorktree(t.Context(), path, opts) })
	if err != nil {
		t.Fatalf("adopt %q: %v; stdout: %s", path, err, out)
	}
	var result createdWorktree
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid adopt JSON %q: %v", out, err)
	}
	return result
}

func workspace(t *testing.T, repo, name, revision string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(repo), "shared", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	vcs(t, repo, "jj", "workspace", "add", "--name="+name, "--revision="+revision, path)
	return path
}

func pathOf(t *testing.T, ref string) string {
	t.Helper()
	out, err := capture(t, func() error { return worktree.Path(t.Context(), ref) })
	if err != nil {
		t.Fatalf("path %q: %v", ref, err)
	}
	return strings.TrimSpace(out)
}

func writeEnvironment(t *testing.T, dir, instance string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("XF_INSTANCE="+instance+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestJJAdoptManagesWorkspaceOutsideConventionalLocation(t *testing.T) {
	repo, marker := fixture(t)
	target := workspace(t, repo, "feature", "@")
	t.Chdir(filepath.Dir(target))
	adopted := adopt(t, filepath.Join("feature", "src"), worktree.AdoptOptions{})
	if adopted.Path != target || adopted.Branch != "feature" || adopted.SourcePath != repo || adopted.SourceBranch == "" || adopted.Instance != "feature" || adopted.Cloned || adopted.CreatedAt.IsZero() {
		t.Fatalf("unexpected adopt result: %+v", adopted)
	}
	for _, dir := range []string{repo, target} {
		t.Chdir(dir)
		if entries := list(t); len(entries) != 1 || entries[target].State != "ok" || entries[target].Branch != "feature" {
			t.Fatalf("list from %s: %+v", dir, entries)
		}
		if got := pathOf(t, "feature"); got != target {
			t.Fatalf("path from %s = %q, want %q", dir, got, target)
		}
	}
	t.Chdir(repo)
	if _, err := capture(t, func() error { return worktree.RemoveWorktree(t.Context(), "feature", worktree.RemoveOptions{}) }); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still exists: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("docker was invoked for a checkout without an environment: %v", err)
	}
	if entries := list(t); len(entries) != 0 {
		t.Fatalf("removed workspace still listed: %+v", entries)
	}
}

func TestJJAdoptInstancePrecedence(t *testing.T) {
	repo, _ := fixture(t)
	target := workspace(t, repo, "feature", "@")
	if err := os.WriteFile(filepath.Join(target, ".env"), []byte("XF_INSTANCE=from-env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := adopt(t, target, worktree.AdoptOptions{}); got.Instance != "from-env" {
		t.Fatalf("instance = %q, want the .env value", got.Instance)
	}
	if got := adopt(t, target, worktree.AdoptOptions{Instance: "custom"}); got.Instance != "custom" {
		t.Fatalf("instance = %q, want the flag value", got.Instance)
	}
	if entries := list(t); len(entries) != 1 || entries[target].Instance != "custom" {
		t.Fatalf("re-adoption did not replace the record: %+v", entries)
	}
}

func TestJJAdoptRefusals(t *testing.T) {
	repo, _ := fixture(t)
	file := filepath.Join(repo, "src", "XF.php")
	empty := workspace(t, repo, "empty", "root()")
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"source checkout", repo, "is the source checkout"},
		{"missing path", filepath.Join(repo, "missing"), "does not exist"},
		{"file", file, "is not a directory"},
		{"not a repository", t.TempDir(), "not a git or Jujutsu repository"},
		{"not XenForo", empty, "src/XF.php not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := capture(t, func() error { return worktree.AdoptWorktree(t.Context(), tc.path, worktree.AdoptOptions{}) })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
	if err := worktree.AdoptWorktree(t.Context(), " ", worktree.AdoptOptions{}); !clierror.IsUsage(err) {
		t.Fatalf("empty path: got %v, want a usage error", err)
	}
	if entries := list(t); len(entries) != 0 {
		t.Fatalf("refused adoptions were registered: %+v", entries)
	}
}

func TestJJRemoveRefusesSharedSourceEnvironment(t *testing.T) {
	repo, marker := fixture(t)
	target := workspace(t, repo, "feature", "@")
	adopt(t, target, worktree.AdoptOptions{})
	writeEnvironment(t, repo, "shared")
	writeEnvironment(t, target, "shared")
	_, err := capture(t, func() error {
		return worktree.RemoveWorktree(t.Context(), "feature", worktree.RemoveOptions{Force: true})
	})
	if err == nil || !strings.Contains(err.Error(), `source environment's instance "shared"`) {
		t.Fatalf("expected shared-environment refusal, got %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("docker was invoked before refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".env")); err != nil {
		t.Fatalf("checkout changed after refusal: %v", err)
	}
	_, err = capture(t, func() error {
		return worktree.RemoveWorktree(t.Context(), "feature", worktree.RemoveOptions{Force: true, KeepContainers: true})
	})
	if err != nil {
		t.Fatalf("remove with --keep-containers: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still exists: %v", err)
	}
}

func TestGitRemoveRefusesReusedAdoptedPath(t *testing.T) {
	repo := gitFixture(t)
	target := filepath.Join(filepath.Dir(repo), "shared", "feature")
	vcs(t, repo, "git", "worktree", "add", "--quiet", "-b", "feature", target)
	adopt(t, target, worktree.AdoptOptions{})
	vcs(t, repo, "git", "worktree", "remove", target)
	vcs(t, repo, "git", "worktree", "add", "--quiet", "-b", "other", target)
	_, err := capture(t, func() error {
		return worktree.RemoveWorktree(t.Context(), "feature", worktree.RemoveOptions{Force: true, KeepContainers: true})
	})
	if err == nil || !strings.Contains(err.Error(), "has other checked out") {
		t.Fatalf("expected refusal for a reused path, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "src", "XF.php")); err != nil {
		t.Fatalf("unrelated worktree was damaged: %v", err)
	}
}

func TestGitRemoveRefusesCheckoutOfAnotherRepository(t *testing.T) {
	repo := gitFixture(t)
	target := filepath.Join(filepath.Dir(repo), "shared", "feature")
	vcs(t, repo, "git", "worktree", "add", "--quiet", "-b", "feature", target)
	adopt(t, target, worktree.AdoptOptions{})
	vcs(t, repo, "git", "worktree", "remove", target)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(filepath.Join(other, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "src", "XF.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcs(t, other, "git", "init", "--quiet", "--initial-branch=main")
	vcs(t, other, "git", "add", ".")
	vcs(t, other, "git", "commit", "--quiet", "-m", "initial")
	vcs(t, other, "git", "worktree", "add", "--quiet", "-b", "feature", target)
	_, err := capture(t, func() error {
		return worktree.RemoveWorktree(t.Context(), "feature", worktree.RemoveOptions{Force: true, KeepContainers: true})
	})
	if err == nil || !strings.Contains(err.Error(), "is not a checkout of the repository") {
		t.Fatalf("expected refusal for another repository's checkout, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "src", "XF.php")); err != nil {
		t.Fatalf("other repository's worktree was damaged: %v", err)
	}
}

func TestGitAdoptAndRemoveLinkedWorktree(t *testing.T) {
	repo := gitFixture(t)
	target := filepath.Join(filepath.Dir(repo), "shared", "feature")
	detached := filepath.Join(filepath.Dir(repo), "shared", "detached")
	vcs(t, repo, "git", "worktree", "add", "--quiet", "-b", "feature", target)
	vcs(t, repo, "git", "worktree", "add", "--quiet", "--detach", detached)
	other := t.TempDir()
	vcs(t, other, "git", "init", "--quiet")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))

	adopted := adopt(t, target, worktree.AdoptOptions{})
	if adopted.Path != target || adopted.Branch != "feature" || adopted.SourcePath != repo || adopted.SourceBranch != "main" || adopted.Instance != "feature" {
		t.Fatalf("unexpected adopt result: %+v", adopted)
	}
	_, err := capture(t, func() error { return worktree.AdoptWorktree(t.Context(), detached, worktree.AdoptOptions{}) })
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("expected detached HEAD refusal, got %v", err)
	}
	if got := pathOf(t, "feature"); got != target {
		t.Fatalf("path = %q, want %q", got, target)
	}
	if _, err := capture(t, func() error { return worktree.RemoveWorktree(t.Context(), "feature", worktree.RemoveOptions{}) }); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree still exists: %v", err)
	}
	if entries := list(t); len(entries) != 0 {
		t.Fatalf("removed worktree still listed: %+v", entries)
	}
	cmd := exec.CommandContext(t.Context(), "git", "--git-dir="+filepath.Join(repo, ".git"), "branch", "--list", "feature")
	if out, err := cmd.Output(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("branch not deleted: %q, %v", out, err)
	}
}
