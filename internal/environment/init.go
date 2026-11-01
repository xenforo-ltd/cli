package environment

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InitOptions contains options for initializing a Docker environment.
type InitOptions struct {
	OverwriteExisting bool
	Contexts          []string
}

// Init initializes the Docker environment in a XenForo directory. It returns
// the paths of any ".default" files written alongside existing, user-modified
// files so callers can notify the user.
func Init(xfDir string, opts InitOptions) (written []string, err error) {
	xfPath := filepath.Join(xfDir, "src", "XF.php")
	if _, statErr := os.Stat(xfPath); errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("not a XenForo directory (src/XF.php not found): %w", statErr)
	}

	extractOpts := extractOptions{
		OverwriteBaseFiles: opts.OverwriteExisting,
		Contexts:           opts.Contexts,
	}

	written, err = extractDockerFilesWithOptions(xfDir, extractOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Docker files: %w", err)
	}

	envPath := filepath.Join(xfDir, ".env")
	if _, statErr := os.Stat(envPath); errors.Is(statErr, fs.ErrNotExist) {
		envDefault, err := GetDockerFile(".env.default")
		if err != nil {
			return nil, fmt.Errorf("failed to read default env: %w", err)
		}

		if err := os.WriteFile(envPath, envDefault, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", envPath, err)
		}

		dirName := filepath.Base(xfDir)
		instanceName := GenerateInstanceName(dirName)

		updates := map[string]string{
			"XF_INSTANCE": instanceName,
		}
		if len(opts.Contexts) > 0 {
			updates["XF_CONTEXTS"] = strings.Join(opts.Contexts, ":")
		}

		if err := WriteEnvFile(envPath, updates); err != nil {
			return nil, fmt.Errorf("failed to update generated .env file: %w", err)
		}
	}

	dockerignorePath := filepath.Join(xfDir, ".dockerignore")
	if _, statErr := os.Stat(dockerignorePath); errors.Is(statErr, fs.ErrNotExist) {
		ignoreDefault, err := GetDockerFile(".dockerignore.default")
		if err != nil {
			return nil, fmt.Errorf("failed to read default dockerignore: %w", err)
		}

		if err := os.WriteFile(dockerignorePath, ignoreDefault, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", dockerignorePath, err)
		}
	}

	return written, nil
}
