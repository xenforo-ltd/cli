// Package update refreshes a project's Docker environment files and images
// from the versions embedded in the CLI.
package update

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// Options specifies what an update should refresh.
type Options struct {
	Path        string
	SkipPull    bool
	SkipBuild   bool
	SkipRestart bool
}

// composeStepCount reports how many of the optional steps will run, so the
// progress totals always match the work shown.
func (o Options) composeStepCount() int {
	count := 0

	if !o.SkipPull {
		count++
	}

	if !o.SkipBuild {
		count++
	}

	if !o.SkipRestart {
		count++
	}

	return count
}

// Run refreshes embedded environment files and optionally pulls, builds, and
// applies images to running containers.
func Run(ctx context.Context, opts Options) error {
	composeSteps := opts.composeStepCount()
	totalSteps := 1 + composeSteps
	step := 0

	nextStep := func(title string) {
		step++
		ui.PrintStep(step, totalSteps, title)
	}

	nextStep("Refreshing environment files")

	written, err := environment.Init(opts.Path, environment.InitOptions{OverwriteExisting: true})
	if err != nil {
		return fmt.Errorf("failed to refresh Docker files: %w", err)
	}

	ui.PrintSuccess("Environment files refreshed")

	for _, path := range written {
		ui.PrintInfo("Default written to " + ui.Path.Render(path))
	}

	if composeSteps == 0 {
		printUpdated(opts.Path, "", "Environment files updated")

		return nil
	}

	if err := environment.CheckDockerRunning(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker is running: %w", err)
	}

	runner, err := environment.NewRunner(opts.Path)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if !opts.SkipPull {
		ui.Println()
		nextStep("Pulling images")

		if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "pull", "--ignore-buildable"); err != nil {
			return fmt.Errorf("failed to pull images: %w", err)
		}
	}

	if !opts.SkipBuild {
		ui.Println()
		nextStep("Building images")

		if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "build", "--pull"); err != nil {
			return fmt.Errorf("failed to build images: %w", err)
		}
	}

	running, err := runner.HasRunningServices(ctx)
	if err != nil {
		return err
	}

	if opts.SkipRestart {
		if running {
			ui.Println()
			ui.PrintWarning("Containers are still running the previous images; run " + ui.Command.Render("xf reboot") + " to apply them")
		}
	} else {
		ui.Println()
		nextStep("Applying to containers")

		if running {
			if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "up", "--detach"); err != nil {
				return fmt.Errorf("failed to restart Docker environment: %w", err)
			}
		} else {
			ui.PrintSubstep("Environment is not running; run " + ui.Command.Render("xf up") + " to start it")
		}
	}

	printUpdated(opts.Path, runner.Instance(), "Environment updated")

	return nil
}

// printUpdated reports the result. Instance is empty when no Docker work ran.
func printUpdated(path, instance, title string) {
	details := []ui.KVPair{ui.KV("Path", ui.Path.Render(ui.ShortHome(path)))}

	if instance != "" {
		details = append(details, ui.KV("Instance", instance))
	}

	ui.Println()
	ui.SuccessBox(title, details)
}
