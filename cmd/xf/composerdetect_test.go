package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestShouldRunComposer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  bool
	}{
		{
			name:  "composer.json present",
			files: []string{"composer.json"},
			want:  true,
		},
		{
			name:  "composer.json and lock present",
			files: []string{"composer.json", "composer.lock"},
			want:  true,
		},
		{
			name:  "no composer files",
			files: nil,
			want:  false,
		},
		{
			name:  "lock without json is not a composer project",
			files: []string{"composer.lock"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			if got := shouldRunComposer(dir); got != tt.want {
				t.Errorf("shouldRunComposer = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShouldRunComposerIgnoresDirectory guards against a directory named
// composer.json being mistaken for a manifest.
func TestShouldRunComposerIgnoresDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "composer.json"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if shouldRunComposer(dir) {
		t.Error("a directory named composer.json must not count as a manifest")
	}
}

func TestPendingAddOnComposerProjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	write := func(rel, content string) {
		t.Helper()

		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	const autoload = `{"composer_autoload": "_vendor/composer"}`

	// Dependencies declared but not installed, as in a fresh worktree.
	write("src/addons/Missing/addon.json", autoload)
	write("src/addons/Missing/composer.json", "{}")

	// Add-ons may be nested one level under a vendor directory.
	write("src/addons/Vendor/Nested/addon.json", autoload)
	write("src/addons/Vendor/Nested/composer.json", "{}")

	// Dependencies already present, as when _vendor is committed.
	write("src/addons/Installed/addon.json", autoload)
	write("src/addons/Installed/composer.json", "{}")
	write("src/addons/Installed/_vendor/composer/autoload_namespaces.php", "<?php")

	// No composer_autoload: XenForo never loads anything.
	write("src/addons/NoAutoload/addon.json", "{}")
	write("src/addons/NoAutoload/composer.json", "{}")

	// No manifest: nothing for Composer to install.
	write("src/addons/NoManifest/addon.json", autoload)

	got := pendingAddOnComposerProjects(root)
	want := []string{"src/addons/Missing", "src/addons/Vendor/Nested"}

	if !slices.Equal(got, want) {
		t.Errorf("pendingAddOnComposerProjects = %v, want %v", got, want)
	}
}

func TestPendingAddOnComposerProjectsWithoutAddOns(t *testing.T) {
	t.Parallel()

	if got := pendingAddOnComposerProjects(t.TempDir()); len(got) != 0 {
		t.Errorf("pendingAddOnComposerProjects = %v, want none", got)
	}
}

// TestPendingAddOnComposerProjectsSkipsUnreadableLoader guards against an error
// other than absence being taken to mean the loader is missing, which would
// run composer install over dependencies that may already be in place.
func TestPendingAddOnComposerProjectsSkipsUnreadableLoader(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not block access on Windows")
	}

	root := t.TempDir()
	addOn := filepath.Join(root, "src", "addons", "Locked")
	vendor := filepath.Join(addOn, "_vendor")

	if err := os.MkdirAll(vendor, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for name, content := range map[string]string{
		"addon.json":    `{"composer_autoload": "_vendor/composer"}`,
		"composer.json": "{}",
	} {
		if err := os.WriteFile(filepath.Join(addOn, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Without search permission on _vendor, stat fails with EACCES rather
	// than reporting the loader missing.
	if err := os.Chmod(vendor, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chmod(vendor, 0o750)
	})

	if _, err := os.Stat(filepath.Join(vendor, "composer")); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Skip("permissions are not enforced for this user")
	}

	if got := pendingAddOnComposerProjects(root); len(got) != 0 {
		t.Errorf("pendingAddOnComposerProjects = %v, want none", got)
	}
}
