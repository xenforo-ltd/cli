package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// ShouldRunComposer reports whether a directory is a Composer project.
//
// Repository checkouts track composer.json, so a fresh worktree has one and its
// dependencies must be installed. Release packages ship vendor/ prebuilt and
// have no manifest, so they are skipped automatically.
func ShouldRunComposer(targetPath string) bool {
	info, err := os.Stat(filepath.Join(targetPath, "composer.json"))

	return err == nil && !info.IsDir()
}

// pendingAddOnComposerProjects returns the add-ons under root whose Composer
// dependencies are declared but not installed, as slash-separated paths
// relative to root.
//
// Some add-ons keep their dependencies in a gitignored _vendor directory, which
// the root composer install does not build. XenForo loads them for every
// installed add-on that sets composer_autoload, so a checkout without them
// fails to boot as soon as the add-on is installed, for example after cloning
// a database.
func pendingAddOnComposerProjects(root string) []string {
	// Add-on IDs are either Name or Vendor/Name.
	var manifests []string

	for _, pattern := range []string{"src/addons/*/addon.json", "src/addons/*/*/addon.json"} {
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		manifests = append(manifests, matches...)
	}

	var pending []string

	for _, manifest := range manifests {
		dir := filepath.Dir(manifest)

		if !ShouldRunComposer(dir) {
			continue
		}

		data, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}

		var addOn struct {
			ComposerAutoload string `json:"composer_autoload"`
		}

		if json.Unmarshal(data, &addOn) != nil || addOn.ComposerAutoload == "" {
			continue
		}

		// XenForo requires this file first, so its absence is what fails. Any
		// other error leaves the state unknown, and installing over a loader
		// that may exist is worse than skipping.
		loader := filepath.Join(dir, filepath.FromSlash(addOn.ComposerAutoload), "autoload_namespaces.php")
		if _, err := os.Stat(loader); !errors.Is(err, fs.ErrNotExist) {
			continue
		}

		rel, err := filepath.Rel(root, dir)
		if err != nil {
			continue
		}

		pending = append(pending, filepath.ToSlash(rel))
	}

	slices.Sort(pending)

	return pending
}

// RunComposerInstall installs Composer dependencies inside the container: the
// root project's, then those of any add-on that still lacks them.
//
// A failing add-on is reported but not fatal. Its dependencies only matter once
// the add-on is installed, and an add-on's manifest can stop resolving on its
// own, such as when Composer starts blocking a pinned version with a security
// advisory. Failing here would make every new environment unusable.
func RunComposerInstall(ctx context.Context, runner *environment.Runner, root string, verbose bool) error {
	if err := runComposerProject(ctx, runner, "", verbose); err != nil {
		return err
	}

	for _, project := range pendingAddOnComposerProjects(root) {
		if err := runComposerProject(ctx, runner, project, verbose); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			ui.PrintHint(fmt.Sprintf(
				"XenForo will fail to start if %s is installed. Fix it, then run %s",
				strings.TrimPrefix(project, "src/addons/"),
				ui.Command.Render("xf composer install --working-dir="+project),
			))
		}
	}

	return nil
}

// runComposerProject runs composer install for one project. An empty project
// is the root; otherwise it is a slash-separated path relative to the root,
// which is also the container's working directory.
func runComposerProject(ctx context.Context, runner *environment.Runner, project string, verbose bool) error {
	args := []string{"composer", "install", "--no-interaction"}
	label := "Composer dependencies"

	if project != "" {
		args = append(args, "--working-dir="+project)
		label = "Composer dependencies for " + strings.TrimPrefix(project, "src/addons/")
	}

	if verbose {
		ui.PrintSubstep("Running " + strings.Join(args, " ") + "...")

		if err := runner.ExecOrRun(ctx, "xf", nil, os.Stdin, os.Stdout, os.Stderr, args...); err != nil {
			if project != "" {
				ui.PrintWarning("Could not install " + label)
			}

			return fmt.Errorf("failed to install %s: %w", label, err)
		}

		return nil
	}

	spinner := ui.NewSpinner("Installing " + label)
	spinner.Start()

	tracker := ui.NewPhaseTrackerWriter(spinner, "Installing "+label, ui.ComposerPhaseRules())

	if err := runner.ExecOrRun(ctx, "xf", nil, os.Stdin, tracker, tracker, args...); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			spinner.Stop()

			return ctxErr
		}

		if project == "" {
			spinner.StopWithMessage("error", "Failed to install "+label)
		} else {
			spinner.StopWithMessage("warning", "Could not install "+label)
		}

		ui.PrintHiddenOutputTail("Composer output", tracker.TailLines())

		return fmt.Errorf("failed to install %s: %w", label, err)
	}

	spinner.StopWithMessage("success", label+" installed")

	return nil
}
