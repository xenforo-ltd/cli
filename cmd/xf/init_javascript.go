package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xenforo-ltd/cli/internal/build"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// initJavaScriptProjects finds source add-ons by their build.json, independently
// of Composer or version control metadata. Release packages omit this file.
// XF comes first so a core build failure stops setup before any add-on builds.
func initJavaScriptProjects(root string) ([]string, error) {
	addOns := os.DirFS(filepath.Join(root, "src", "addons"))
	var projects []string

	for _, pattern := range []string{"*/build.json", "*/*/build.json"} {
		matches, err := fs.Glob(addOns, pattern)
		if err != nil {
			return nil, fmt.Errorf("finding JavaScript build configurations: %w", err)
		}

		for _, manifest := range matches {
			info, err := fs.Stat(addOns, manifest)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("checking JavaScript build configuration %s: %w", manifest, err)
			}
			if info.IsDir() {
				continue
			}

			projects = append(projects, path.Dir(manifest))
		}
	}

	slices.SortFunc(projects, func(a, b string) int {
		if a == "XF" && b != "XF" {
			return -1
		}
		if b == "XF" && a != "XF" {
			return 1
		}
		return strings.Compare(a, b)
	})

	return projects, nil
}

// runInitJavaScript builds only JavaScript, never release tasks or configured
// commands. Add-on failures warn and continue; core failures and cancellation
// stop setup. Both init modes call this before any container startup.
func runInitJavaScript(ctx context.Context, opts *InitOptions, step, totalSteps int) error {
	const title = "Building JavaScript"
	if opts.SkipBuildJS {
		printSkippedStep(step, totalSteps, title, "--skip-build-js")
		return nil
	}

	projects, err := initJavaScriptProjects(opts.TargetPath)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		printSkippedStep(step, totalSteps, title, "build.json not present")
		return nil
	}

	ui.PrintStep(step, totalSteps, title)
	for _, project := range projects {
		label := "JavaScript for " + project
		spinner := ui.NewSpinner("Building " + label)
		spinner.Start()

		if err := build.BuildJavaScript(ctx, opts.TargetPath, project); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				spinner.Stop()
				return ctxErr
			}
			if project == "XF" {
				spinner.StopWithMessage("error", "Failed to build "+label)
				return fmt.Errorf("failed to build %s: %w", label, err)
			}

			spinner.StopWithMessage("warning", "Could not build "+label)
			ui.PrintWarning(err.Error())
			ui.PrintHint(fmt.Sprintf(
				"%s may not work without its JavaScript. Fix it, then run %s",
				project, ui.Command.Render("xf build js "+project),
			))
			continue
		}

		spinner.StopWithMessage("success", label+" built")
	}

	return nil
}
