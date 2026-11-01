// Package xf provides utilities for XenForo installation detection and management.
package xf

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetXenForoDir finds the XenForo root directory by traversing up from startDir.
// It also checks the XF_DIR environment variable as a fallback.
func GetXenForoDir(startDir string) (string, error) {
	dir := filepath.Clean(startDir)
	for {
		xfPath := filepath.Join(dir, "src", "XF.php")
		if _, err := os.Stat(xfPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	if xfDir := filepath.Clean(os.Getenv("XF_DIR")); xfDir != "." {
		if _, err := os.Stat(filepath.Join(xfDir, "src", "XF.php")); err == nil {
			return xfDir, nil
		}
	}

	return "", fmt.Errorf("not in a XenForo directory and XF_DIR not set: %w", errInvalidInput)
}
