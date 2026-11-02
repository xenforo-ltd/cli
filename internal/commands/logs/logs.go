package logs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func Run(ctx context.Context, args []string, follow bool) error {
	xfDir, services, err := passthrough.Resolve(args)
	if err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if !follow {
		target := "all services"
		if len(services) > 0 {
			target = strings.Join(services, ", ")
		}

		ui.PrintInfo("Showing logs for " + ui.Bold.Render(target))
	}

	composeArgs := []string{"logs"}
	if follow {
		composeArgs = append(composeArgs, "--follow")
	}

	composeArgs = append(composeArgs, services...)

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, composeArgs...); err != nil {
		return fmt.Errorf("failed to show container logs: %w", err)
	}

	return nil
}
