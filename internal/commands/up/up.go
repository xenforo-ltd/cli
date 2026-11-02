package up

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// Run checks Docker availability and starts the environment, optionally detached.
func Run(ctx context.Context, args []string, detach bool) error {
	xfDir, err := passthrough.XenForoDir(args)
	if err != nil {
		return err
	}

	if err := environment.CheckDockerRunning(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker is running: %w", err)
	}

	if err := environment.CheckDockerComposeAvailable(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker Compose is available: %w", err)
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintInfo("Starting Docker environment " + ui.Bold.Render(runner.Instance()) + " " + ui.Muted.Render("("+ui.ShortHome(xfDir)+")"))

	composeArgs := []string{"up"}
	if detach {
		composeArgs = append(composeArgs, "--detach")
	}

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, composeArgs...); err != nil {
		return fmt.Errorf("failed to start Docker environment: %w", err)
	}

	details := []ui.KVPair{}
	if url, err := runner.GetURL(ctx); err == nil {
		details = append(details, ui.KV("URL", ui.URL.Render(url)))
	} else {
		ui.PrintWarning("Could not determine the site URL")
	}

	ui.Println()
	ui.SuccessBox("Docker environment started", details)

	return nil
}
