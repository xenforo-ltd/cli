package compose

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
)

func Run(ctx context.Context, args []string) error {
	xfDir, composeArgs, err := passthrough.Resolve(args)
	if err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, composeArgs...); err != nil {
		return clierror.Passthrough(err, "failed to run docker compose")
	}

	return nil
}
