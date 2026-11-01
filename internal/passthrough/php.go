package passthrough

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func PHP(ctx context.Context, args []string, debug bool) error {
	xfDir, phpArgs, err := Resolve(args)
	if err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	var env map[string]string

	if debug {
		// Only the first argument is echoed. The remainder routinely carries
		// inline code (-r) or tokens, and this line ends up in terminal
		// scrollback and CI logs.
		label := "Xdebug enabled"
		if len(phpArgs) > 0 {
			label += ": " + ui.Command.Render("php "+phpArgs[0])
		}

		ui.PrintInfo(label)

		env = map[string]string{"XDEBUG_SESSION": "1"}
	}

	err = runner.ExecOrRun(ctx, "xf", env, os.Stdin, os.Stdout, os.Stderr, append([]string{"php"}, phpArgs...)...)

	return clierror.Passthrough(err, "failed to run PHP")
}
