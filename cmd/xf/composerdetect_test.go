package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/extract"
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

// TestComposerDetectionAgreesBeforeAndAfterExtraction is a regression test
// for a step-count bug: plannedInitSteps was originally seeded by calling
// shouldRunComposer(opts.TargetPath) - a filesystem check - before download
// or extraction had put anything on disk, while the later composer-install
// gate in executeInit re-ran the same filesystem check after extraction. For
// a repository checkout (the normal case shouldRunComposer documents),
// composer.json only exists inside the downloaded package, so the
// pre-extraction check always said "no composer step" while the
// post-extraction gate said "yes" - printing one more step than the planned
// total ([8/7]).
//
// executeInit now resolves hasComposer once, from the archive itself via
// extract.ContainsUploadFile, before either decision is made, and threads
// that single value into both plannedInitSteps and the later gate. This test
// pins the two facts the fix depends on: shouldRunComposer on an empty
// (pre-extraction) target directory is false for a package that nonetheless
// contains composer.json, and extract.ContainsUploadFile detects it directly
// from the archive without needing extraction first - so the plan can be
// correct even for a target directory that doesn't have the file yet.
func TestComposerDetectionAgreesBeforeAndAfterExtraction(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	if shouldRunComposer(targetDir) {
		t.Fatal("expected an empty pre-extraction target directory to report no composer.json")
	}

	zipPath := filepath.Join(t.TempDir(), "xenforo-repo-checkout.zip")

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for name, content := range map[string]string{
		"upload/src/XF.php":    "<?php // XF stub",
		"upload/composer.json": `{"name":"xenforo/xenforo"}`,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}

		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	if err := os.WriteFile(zipPath, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip file: %v", err)
	}

	// This is the pre-extraction signal executeInit now uses for both the
	// plan and the later gate.
	hasComposer, err := extract.ContainsUploadFile(zipPath, "composer.json")
	if err != nil {
		t.Fatalf("ContainsUploadFile: %v", err)
	}

	if !hasComposer {
		t.Fatal("expected the archive-level check to detect composer.json before extraction")
	}

	if plannedInitSteps(InitOptions{}, hasComposer) != 8 {
		t.Fatalf("plannedInitSteps disagreed with the archive-level hasComposer signal")
	}

	// Simulate extraction landing the file on disk, then confirm the
	// post-extraction filesystem check agrees with the pre-extraction
	// archive check - demonstrating the single hasComposer value executeInit
	// threads through both call sites is valid at either point in time, so
	// there is no longer a place where the two disagree.
	if err := os.WriteFile(filepath.Join(targetDir, "composer.json"), []byte(`{"name":"xenforo/xenforo"}`), 0o600); err != nil {
		t.Fatalf("simulate extraction: %v", err)
	}

	if shouldRunComposer(targetDir) != hasComposer {
		t.Fatal("post-extraction shouldRunComposer must agree with the pre-extraction hasComposer signal")
	}
}
