package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// destroyWorktreeEnvironment removes a worktree's containers and volumes.
//
// A worktree that was never set up has no compose configuration, which is not
// an error: there is simply nothing to tear down. Other inspection failures
// stop removal to avoid stranding containers and volumes.
func destroyWorktreeEnvironment(ctx context.Context, worktreePath string) error {
	runner, err := environment.NewRunner(worktreePath)
	if err != nil {
		if errors.Is(err, environment.ErrEnvNotInitialized) || errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to inspect the worktree environment: %w", err)
	}

	spinner := ui.NewSpinner("Removing containers and volumes")
	spinner.Start()

	if err := runner.Destroy(ctx); err != nil {
		spinner.Stop()

		return fmt.Errorf("failed to remove the worktree environment: %w", err)
	}

	spinner.StopWithMessage("success", "Containers and volumes removed")

	return nil
}

// sourceIsInstalled reports whether the source checkout holds a XenForo
// installation that can be cloned.
//
// A checkout that has never been installed has no database or attachments to
// copy, so a new worktree gets a fresh install instead. Both the install lock
// and Compose configuration must exist; inspection failures other than absence
// are reported rather than mistaken for a fresh installation.
func sourceIsInstalled(sourcePath string) (bool, error) {
	markers := []string{
		filepath.Join(sourcePath, "internal_data", "install-lock.php"),
		filepath.Join(sourcePath, "compose.yaml"),
	}

	for _, marker := range markers {
		if _, err := os.Stat(marker); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}

			return false, fmt.Errorf("failed to inspect %s: %w", marker, err)
		}
	}

	return true, nil
}
