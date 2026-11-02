package exec

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
)

// Run executes a command in a Docker Compose service with standard streams attached.
func Run(ctx context.Context, args []string) error {
	xfDir, execArgs, err := passthrough.Resolve(args)
	if err != nil {
		return err
	}

	if err := validateExecInvocation(execArgs); err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, append([]string{"exec"}, execArgs...)...); err != nil {
		return clierror.Passthrough(err, "failed to execute command in service "+execArgs[0])
	}

	return nil
}

// validateExecInvocation requires both a service and a command.
func validateExecInvocation(execArgs []string) error {
	if len(execArgs) < 2 {
		return clierror.Usage(fmt.Errorf("exec requires <service> <command> [args...]: %w", clierror.ErrInvalidInput))
	}

	return nil
}
