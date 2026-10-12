package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/worktree"
)

// useTempRegistry points the worktree registry at a temporary config directory
// and returns it.
func useTempRegistry(t *testing.T) *worktree.Registry {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))

	registry, err := worktree.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	return registry
}

// TestWorktreeNameTakenBySourceInstance covers a source whose environment is
// configured but has never started, so Docker has no project for it yet.
func TestWorktreeNameTakenBySourceInstance(t *testing.T) {
	registry := useTempRegistry(t)

	taken, err := worktreeNameTaken(t.Context(), registry, "feature", "feature")
	if err != nil {
		t.Fatalf("worktreeNameTaken: %v", err)
	}

	if !taken {
		t.Error("the source's configured instance must not be reused")
	}
}

// TestWorktreeNameTakenByRegistryEntry covers a worktree with no Docker
// project, such as one created with --no-setup or still being created. The
// entry reserves its name even though its directory does not exist.
func TestWorktreeNameTakenByRegistryEntry(t *testing.T) {
	registry := useTempRegistry(t)

	if err := registry.Reserve(worktree.Entry{
		WorktreePath: filepath.Join(t.TempDir(), "absent"),
		Instance:     "feature",
	}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	taken, err := worktreeNameTaken(t.Context(), registry, "", "feature")
	if err != nil {
		t.Fatalf("worktreeNameTaken: %v", err)
	}

	if !taken {
		t.Error("a registered instance must not be reused")
	}
}

// TestWorktreeNameTakenFailsOnACorruptRegistry covers the registry being what
// keeps names unique: unreadable must not mean available.
func TestWorktreeNameTakenFailsOnACorruptRegistry(t *testing.T) {
	registry := useTempRegistry(t)

	path, err := worktree.RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = worktreeNameTaken(t.Context(), registry, "", "feature")
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("worktreeNameTaken = %v, want a corrupt registry error", err)
	}
}

func TestConfiguredInstance(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := configuredInstance(dir)
	if err != nil || got != "" {
		t.Errorf("configuredInstance without an environment = %q, %v; want empty", got, err)
	}

	for name, content := range map[string]string{"compose.yaml": "services: {}\n", ".env": "XF_INSTANCE=main\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err = configuredInstance(dir)
	if err != nil || got != "main" {
		t.Errorf("configuredInstance = %q, %v; want main", got, err)
	}
}
