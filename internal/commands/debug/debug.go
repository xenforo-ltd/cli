package debug

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

func Run(ctx context.Context, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine the current directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return fmt.Errorf("failed to find XenForo directory: %w", err)
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	// Only the command name is echoed. Arguments routinely carry passwords and
	// tokens, and this line ends up in terminal scrollback and CI logs.
	ui.PrintInfo("Xdebug enabled: " + ui.Command.Render(args[0]))

	if err := runner.ExecOrRun(ctx, "xf", map[string]string{"XDEBUG_SESSION": "1"}, os.Stdin, os.Stdout, os.Stderr, append([]string{"php", "cmd.php"}, args...)...); err != nil {
		// A cancelled command reports an exit status of -1, which is not a
		// status any caller should receive. Report the cancellation itself so
		// the interrupt exit code is used.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return clierror.Passthrough(err, "failed to run the command with Xdebug")
	}

	return nil
}
