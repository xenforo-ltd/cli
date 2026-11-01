package passthrough

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/xf"
)

// XenForoDir gets the XenForo directory from args or auto-detects.
func XenForoDir(args []string) (string, error) {
	if len(args) > 0 {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}

		xfPath := filepath.Join(absPath, "src", "XF.php")
		if _, err := os.Stat(xfPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", clierror.MarkAs(os.ErrNotExist, "not a XenForo installation: %s (no src/XF.php found)", absPath)
			}

			return "", fmt.Errorf("cannot access %s: %w", absPath, err)
		}

		return absPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to find XenForo directory: %w", err)
	}

	return xfDir, nil
}

func Resolve(args []string) (string, []string, error) {
	if len(args) > 0 {
		if dir, err := XenForoDir(args[:1]); err == nil {
			return dir, stripSeparator(args[1:]), nil
		}
	}

	xfDir, err := XenForoDir(nil)
	if err != nil {
		return "", nil, err
	}

	return xfDir, stripSeparator(args), nil
}

func stripSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}

	return args
}
