package reboot

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func Run(ctx context.Context, args []string) error {
	xfDir, err := passthrough.XenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintStep(1, 2, "Stopping "+runner.Instance())

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "down"); err != nil {
		return fmt.Errorf("failed to stop Docker environment: %w", err)
	}

	ui.PrintStep(2, 2, "Starting "+runner.Instance())

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "up", "--detach"); err != nil {
		return fmt.Errorf("failed to start Docker environment: %w", err)
	}

	details := []ui.KVPair{}
	if url, err := runner.GetURL(ctx); err == nil {
		details = append(details, ui.KV("URL", ui.URL.Render(url)))
	} else {
		ui.PrintWarning("Could not determine the site URL")
	}

	ui.Println()
	ui.SuccessBox("Docker environment restarted", details)

	return nil
}
