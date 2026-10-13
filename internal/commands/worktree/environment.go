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

// sharedEnvironment returns the instance name that target shares with source,
// or "" if target has its own.
func sharedEnvironment(source, target string) (string, error) {
	sourceInstance, err := configuredInstance(source)
	if err != nil {
		return "", err
	}

	if sourceInstance == "" {
		return "", nil
	}

	targetInstance, err := configuredInstance(target)
	if err != nil {
		return "", err
	}

	if targetInstance != sourceInstance {
		return "", nil
	}

	return sourceInstance, nil
}

// refuseSharedEnvironment returns an error if target shares the instance of
// source.
func refuseSharedEnvironment(source, target string) error {
	shared, err := sharedEnvironment(source, target)
	if err != nil {
		return err
	}

	if shared == "" {
		return nil
	}

	return fmt.Errorf(
		"refusing to remove the environment at %s: it uses the source environment's instance %q, so tearing it down would delete the source's database and volumes. Use --keep-containers to remove only the checkout, or give the checkout its own XF_INSTANCE",
		target, shared,
	)
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

// requireXenForoCheckout returns an error unless root contains a regular file
// src/XF.php.
func requireXenForoCheckout(root string) error {
	xfPath := filepath.Join(root, "src", "XF.php")

	info, err := os.Stat(xfPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: src/XF.php not found in %s", errNotXenForo, root)
		}

		return fmt.Errorf("failed to inspect %s: %w", xfPath, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", errNotXenForo, xfPath)
	}

	return nil
}
