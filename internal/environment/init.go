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
	if _, statErr := os.Stat(xfPath); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("not a XenForo directory (src/XF.php not found): %w", statErr)
		}

		return nil, fmt.Errorf("cannot access src/XF.php: %w", statErr)
	}

	// Extraction creates .env from .env.default, so its presence afterwards
	// cannot distinguish a new environment from one the user already had.
	// Record it beforehand to avoid overwriting a user's configuration.
	envPath := filepath.Join(xfDir, ".env")
	newEnv := false
	if _, statErr := os.Stat(envPath); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("cannot access %s: %w", envPath, statErr)
		}

		newEnv = true
	}

	extractOpts := extractOptions{
		overwriteBaseFiles: opts.OverwriteExisting,
	}

	written, err = extractDockerFilesWithOptions(xfDir, extractOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Docker files: %w", err)
	}

	if newEnv {
		updates := map[string]string{
			"XF_INSTANCE": GenerateInstanceName(filepath.Base(xfDir)),
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
