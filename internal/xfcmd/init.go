// Package xfcmd provides high-level commands for managing XenForo installations.
package xfcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/docker"
	"github.com/xenforo-ltd/cli/internal/xf"
)

// InitOptions contains options for initializing a Docker environment.
type InitOptions struct {
	OverwriteExisting bool
	Contexts          []string
}

// Init initializes the Docker environment in a XenForo directory.
func Init(xfDir string, opts InitOptions) error {
	xfPath := filepath.Join(xfDir, "src", "XF.php")
	if _, err := os.Stat(xfPath); os.IsNotExist(err) {
		return fmt.Errorf("not a XenForo directory (src/XF.php not found): %w", err)
	}

	extractOpts := docker.ExtractOptions{
		OverwriteBaseFiles: opts.OverwriteExisting,
		Contexts:           opts.Contexts,
	}

	if err := docker.ExtractDockerFilesWithOptions(xfDir, extractOpts); err != nil {
		return fmt.Errorf("failed to extract Docker files: %w", err)
	}

	envPath := filepath.Join(xfDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envDefault, err := docker.GetEnvDefault()
		if err != nil {
			return fmt.Errorf("failed to read default env: %w", err)
		}

		if err := os.WriteFile(envPath, envDefault, 0o600); err != nil {
			return fmt.Errorf("failed to write .env file: %w", err)
		}

		dirName := filepath.Base(xfDir)
		instanceName := xf.GenerateInstanceName(dirName)

		updates := map[string]string{
			"XF_INSTANCE": instanceName,
		}
		if len(opts.Contexts) > 0 {
			updates["XF_CONTEXTS"] = strings.Join(opts.Contexts, ":")
		}

		if err := xf.WriteEnvFile(envPath, updates); err != nil {
			return fmt.Errorf("failed to update generated .env file: %w", err)
		}
	}

	dockerignorePath := filepath.Join(xfDir, ".dockerignore")
	if _, err := os.Stat(dockerignorePath); os.IsNotExist(err) {
		ignoreDefault, err := docker.GetDockerIgnoreDefault()
		if err != nil {
			return fmt.Errorf("failed to read default dockerignore: %w", err)
		}

		if err := os.WriteFile(dockerignorePath, ignoreDefault, 0o600); err != nil {
			return fmt.Errorf("failed to write .dockerignore file: %w", err)
		}
	}

	return nil
}

// InitExisting initializes Docker environment in an existing XenForo directory.
func InitExisting(xfDir string, opts InitOptions) error {
	return Init(xfDir, opts)
}

// Update updates the Docker environment by re-initializing with latest embedded files.
func Update(xfDir string) error {
	return Init(xfDir, InitOptions{OverwriteExisting: true})
}

// Prune removes unused Docker resources.
func Prune() error {
	return nil
}
