package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/passthrough"
)

// getBuildTarget resolves the installation and an explicit or enclosing add-on.
// Without an enclosing addon.json under src/addons, the target defaults to XF.
func getBuildTarget(args []string) (string, string, error) {
	xfDir, err := passthrough.XenForoDir(nil)
	if err != nil {
		return "", "", err
	}
	xfDir, err = filepath.Abs(xfDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving XenForo root: %w", err)
	}
	xfDir, err = filepath.EvalSymlinks(xfDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving XenForo root: %w", err)
	}
	if len(args) > 0 {
		return xfDir, args[0], nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("reading current directory: %w", err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolving current directory: %w", err)
	}
	addOnsDir := filepath.Join(xfDir, "src", "addons")
	for {
		rel, err := filepath.Rel(addOnsDir, dir)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			break
		}
		metadataPath := filepath.Join(dir, "addon.json")
		info, err := os.Stat(metadataPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("reading %s: %w", metadataPath, err)
		}
		addOnID := filepath.ToSlash(rel)
		if err == nil && info.Mode().IsRegular() && strings.Count(addOnID, "/") <= 1 {
			return xfDir, addOnID, nil
		}
		dir = filepath.Dir(dir)
	}
	return xfDir, "XF", nil
}
