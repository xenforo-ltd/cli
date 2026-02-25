package testutils

import (
	"os"
	"path/filepath"
	"testing"
)

func SetupXenForoDir(t *testing.T) string {
	t.Helper()
	t.Setenv("XF_DIR", "")

	root := t.TempDir()

	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(xfFile, []byte("<?php"), 0o644); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	return root
}
