package passthrough

import (
	"context"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// PHP runs PHP in the installation's Docker environment. In debug mode it
// enables Xdebug and displays only the first PHP argument to avoid exposing
// inline code or tokens in terminal scrollback and logs.
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
