// Package testutils provides testing utilities for the CLI.
package testutils

import (
	"os"
	"path/filepath"
	"testing"
)

// SetupXenForoDir creates a temporary XenForo directory structure for testing.
func SetupXenForoDir(t *testing.T) string {
	t.Helper()
	t.Setenv("XF_DIR", "")

	root := t.TempDir()
	xfFile := filepath.Join(root, "src", "XF.php")

	if err := os.MkdirAll(filepath.Dir(xfFile), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(xfFile, []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	return root
}
