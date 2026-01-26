package xfcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xf/internal/embed"
	"xf/internal/errors"
	"xf/internal/xf"
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
		return errors.New(errors.CodeInvalidInput, "not a XenForo directory (src/XF.php not found)")
	}

	extractOpts := embed.ExtractOptions{
		OverwriteBaseFiles: opts.OverwriteExisting,
		Contexts:           opts.Contexts,
	}

	if err := embed.ExtractDockerFilesWithOptions(xfDir, extractOpts); err != nil {
		return errors.Wrap(errors.CodeFileWriteFailed, "failed to extract Docker files", err)
	}

	envPath := filepath.Join(xfDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envDefault, err := embed.GetEnvDefault()
		if err != nil {
			return errors.Wrap(errors.CodeFileReadFailed, "failed to read default env", err)
		}

		dirName := filepath.Base(xfDir)
		instanceName := xf.GenerateInstanceName(dirName)
		envContent := strings.ReplaceAll(string(envDefault), "XF_INSTANCE=xf", fmt.Sprintf("XF_INSTANCE=%s", instanceName))

		if len(opts.Contexts) > 0 {
			envContent = strings.ReplaceAll(envContent,
				"XF_CONTEXTS=caddy:mysql:development:caddy-development:redis:mailpit",
				fmt.Sprintf("XF_CONTEXTS=%s", strings.Join(opts.Contexts, ":")))
		}

		if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
			return errors.Wrap(errors.CodeFileWriteFailed, "failed to write .env file", err)
		}
	}

	dockerignorePath := filepath.Join(xfDir, ".dockerignore")
	if _, err := os.Stat(dockerignorePath); os.IsNotExist(err) {
		ignoreDefault, err := embed.GetDockerIgnoreDefault()
		if err != nil {
			return errors.Wrap(errors.CodeFileReadFailed, "failed to read default dockerignore", err)
		}

		if err := os.WriteFile(dockerignorePath, ignoreDefault, 0644); err != nil {
			return errors.Wrap(errors.CodeFileWriteFailed, "failed to write .dockerignore file", err)
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

func Prune() error {
	return nil
}
