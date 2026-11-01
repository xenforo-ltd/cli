package down

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

	ui.PrintInfo("Stopping Docker environment " + ui.Bold.Render(runner.Instance()))

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, downComposeArgs()...); err != nil {
		return fmt.Errorf("failed to stop Docker environment: %w", err)
	}

	ui.Println()
	ui.SuccessBox("Docker environment stopped", nil)

	return nil
}

// downComposeArgs lists the compose arguments for stopping an environment.
// It deliberately omits --volumes so the database and other volume data
// survive a later start, unlike Destroy.
func downComposeArgs() []string {
	return []string{"down"}
}
