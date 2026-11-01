package root

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

// commandFunc builds an external command. It is a seam for tests, and takes a
// context so that cancellation reaches the child process.
type commandFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

func Run(ctx context.Context, args []string) error {
	return runAsXenForoCommand(ctx, args, exec.CommandContext)
}

func runAsXenForoCommand(ctx context.Context, args []string, cmdFn commandFunc) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine the current directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return clierror.WithHint(
			clierror.MarkAs(err, "unknown command: %s (not in a XenForo directory)", args[0]),
			"Run "+ui.Command.Render("xf --help")+" to see available commands",
		)
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		if errors.Is(err, environment.ErrEnvNotInitialized) {
			return runAsLocalXenForoCommand(ctx, xfDir, args, cmdFn)
		}

		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if err := runner.ExecOrRun(ctx, "xf", nil, os.Stdin, os.Stdout, os.Stderr, append([]string{"php", "cmd.php"}, args...)...); err != nil {
		return clierror.Passthrough(err, fmt.Sprintf("failed to run XenForo command %q", args[0]))
	}

	return nil
}

func runAsLocalXenForoCommand(ctx context.Context, xfDir string, args []string, cmdFn commandFunc) error {
	cmdArgs := append([]string{"cmd.php"}, args...)
	cmd := cmdFn(ctx, "php", cmdArgs...)
	cmd.Dir = xfDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// The child is killed when the context ends, so report why it stopped
		// rather than the resulting signal.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if errors.Is(err, exec.ErrNotFound) {
			return clierror.WithHint(
				clierror.MarkAs(err, "PHP is not installed or not in your PATH"),
				"Install PHP or run this inside a started environment ("+ui.Command.Render("xf up")+")",
			)
		}

		return clierror.Passthrough(err, "local XenForo command failed")
	}

	return nil
}
