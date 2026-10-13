package worktree_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/commands/worktree"
)

type createdWorktree struct {
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	SourcePath   string    `json:"source_path"`
	SourceBranch string    `json:"source_branch"`
	Instance     string    `json:"instance"`
	Cloned       bool      `json:"cloned"`
	CreatedAt    time.Time `json:"created_at"`
}

type listedWorktree struct {
	Path     string `json:"worktree_path"`
	Branch   string `json:"branch"`
	Instance string `json:"instance"`
	State    string `json:"state"`
}

func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	oldWriter := lipgloss.Writer
	writer := *oldWriter
	writer.Forward = f
	os.Stdout = f
	lipgloss.Writer = &writer
	defer func() { os.Stdout = old; lipgloss.Writer = oldWriter; _ = f.Close() }()
	err = fn()
	data, readErr := os.ReadFile(f.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), err
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker requires a POSIX shell")
	}
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	repo := filepath.Join(root, "repo")
	for _, dir := range []string{home, bin, repo} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(home, "jj.toml")
	if err := os.WriteFile(config, []byte("[user]\nname = 'Test User'\nemail = 'test@example.com'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "docker-called")
	docker := "#!/bin/sh\nprintf called > \"$DOCKER_MARKER\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(docker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	t.Setenv("JJ_CONFIG", config)
	t.Setenv("DOCKER_MARKER", marker)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd := exec.CommandContext(t.Context(), "jj", "git", "init", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "XF.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	repo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo, marker
}

func create(t *testing.T, branch string) createdWorktree {
	t.Helper()
	out, err := capture(t, func() error {
		return worktree.CreateWorktree(t.Context(), []string{branch}, worktree.CreateOptions{NoSetup: true, JSON: true, Fresh: true, NoUp: true})
	})
	if err != nil {
		t.Fatalf("create %q: %v; stdout: %s", branch, err, out)
	}
	var result createdWorktree
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid create JSON %q: %v", out, err)
	}
	return result
}

func list(t *testing.T) map[string]listedWorktree {
	t.Helper()
	out, err := capture(t, func() error { return worktree.List(t.Context(), true) })
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var entries []listedWorktree
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("invalid list JSON %q: %v", out, err)
	}
	byPath := make(map[string]listedWorktree, len(entries))
	for _, entry := range entries {
		if _, exists := byPath[entry.Path]; exists {
			t.Fatalf("duplicate worktree path in list: %+v", entries)
		}
		byPath[entry.Path] = entry
	}
	return byPath
}

func TestCreatePreflightWithoutRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name string
		args []string
		opts worktree.CreateOptions
		want string
	}{
		{"json requires no setup", []string{"feature"}, worktree.CreateOptions{JSON: true}, "--json requires --no-setup"},
		{"missing branch", nil, worktree.CreateOptions{NoSetup: true}, "branch name is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := worktree.CreateWorktree(t.Context(), tc.args, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestJJCreateListPathAndPrune(t *testing.T) {
	repo, _ := fixture(t)
	first := create(t, "one/feature")
	second := create(t, "two/feature")
	if first.Path != filepath.Join(repo+".worktrees", "feature") || second.Path != filepath.Join(repo+".worktrees", "feature-2") {
		t.Fatalf("collision paths: %q, %q", first.Path, second.Path)
	}
	if first.Branch != "one/feature" || first.SourcePath != repo || first.SourceBranch == "" || first.Instance != "feature" || first.Cloned || first.CreatedAt.IsZero() || second.Instance != "feature-2" {
		t.Fatalf("unexpected create results: %+v, %+v", first, second)
	}
	for _, ref := range []string{"one/feature", "feature"} {
		out, err := capture(t, func() error { return worktree.Path(t.Context(), ref) })
		if err != nil || strings.TrimSpace(out) != first.Path {
			t.Fatalf("path %q: %q, %v", ref, out, err)
		}
	}
	entries := list(t)
	if len(entries) != 2 || entries[first.Path].State != "ok" || entries[second.Path].State != "ok" {
		t.Fatalf("unexpected list: %+v", entries)
	}
	if err := os.RemoveAll(first.Path); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error { return worktree.Path(t.Context(), "one/feature") })
	if err != nil || strings.TrimSpace(out) != first.Path {
		t.Fatalf("path after external deletion: %q, %v", out, err)
	}
	entries = list(t)
	if len(entries) != 2 || entries[first.Path].State != "missing" || entries[second.Path].State != "ok" {
		t.Fatalf("unexpected list after deletion: %+v", entries)
	}
	if _, err := capture(t, worktree.Prune); err != nil {
		t.Fatalf("prune: %v", err)
	}
	entries = list(t)
	if len(entries) != 1 || entries[second.Path].State != "ok" {
		t.Fatalf("unexpected list after prune: %+v", entries)
	}
}

func TestJJRemoveDirtyAndForce(t *testing.T) {
	repo, marker := fixture(t)
	w := create(t, "dirty")
	sentinel := filepath.Join(w.Path, "valuable.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return worktree.RemoveWorktree(t.Context(), "dirty", worktree.RemoveOptions{}) })
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") || !strings.Contains(err.Error(), "valuable.txt") {
		t.Fatalf("expected dirty-workspace refusal, got %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("docker was invoked before refusal: %v", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep me" {
		t.Fatalf("sentinel changed: %q, %v", data, err)
	}
	unowned := filepath.Join(repo+".worktrees", "unowned")
	if err := os.Mkdir(unowned, 0o755); err != nil {
		t.Fatal(err)
	}
	unownedFile := filepath.Join(unowned, "sentinel")
	if err := os.WriteFile(unownedFile, []byte("unowned"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = capture(t, func() error {
		return worktree.RemoveWorktree(t.Context(), "unowned", worktree.RemoveOptions{Force: true, KeepContainers: true})
	})
	if err == nil {
		t.Fatal("forced removal unexpectedly accepted unowned directory")
	}
	if data, err := os.ReadFile(unownedFile); err != nil || string(data) != "unowned" {
		t.Fatalf("unowned directory was damaged: %q, %v", data, err)
	}
	_, err = capture(t, func() error {
		return worktree.RemoveWorktree(t.Context(), "dirty", worktree.RemoveOptions{Force: true, KeepContainers: true})
	})
	if err != nil {
		t.Fatalf("force remove: %v", err)
	}
	if _, err := os.Stat(w.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still exists: %v", err)
	}
	if entries := list(t); len(entries) != 0 {
		t.Fatalf("removed workspace still listed: %+v", entries)
	}
}

func TestListExplainsCorruptRegistry(t *testing.T) {
	fixture(t)

	dir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config directory: %v", err)
	}
	path := filepath.Join(dir, "xf", "worktrees.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}

	err = worktree.List(t.Context(), true)
	if err == nil {
		t.Fatal("corrupt registry was accepted")
	}
	if hint := clierror.HintOf(err); !strings.Contains(hint, "delete") {
		t.Fatalf("hint = %q", hint)
	}
}
