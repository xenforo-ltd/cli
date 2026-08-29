package main

import (
	"os"
	"path/filepath"
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
