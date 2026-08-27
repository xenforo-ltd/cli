package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var phpCmd = &cobra.Command{
	Use:   "php [path] [args...]",
	Short: "Run PHP commands",
	Long: `Run PHP in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.

Everything after 'php' is passed to PHP, including flags. Give xf's own flags
before the command name, and use 'xf help php' for this help text.`,
	Example: `  # Check PHP version
  xf php -v

  # Run a PHP script
  xf php my-script.php

  # Run PHP in specific directory
  xf php ./my-project -v

  # Enable xf verbose output (before the command name)
  xf --verbose php my-script.php`,
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(0),
	GroupID:            "run",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPHP(cmd.Context(), args, false)
	},
}

var phpDebugCmd = &cobra.Command{
	Use:   "php-debug [path] [args...]",
	Short: "Run PHP with Xdebug",
	Long: `Run PHP with Xdebug enabled in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.
All arguments are passed to PHP.`,
	Example: `  # Run PHP script with Xdebug
  xf php-debug my-script.php`,
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(0),
	GroupID:            "run",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPHP(cmd.Context(), args, true)
	},
}

func init() {
	rootCmd.AddCommand(phpCmd)
	rootCmd.AddCommand(phpDebugCmd)
}

func runPHP(ctx context.Context, args []string, debug bool) error {
	xfDir, phpArgs, err := resolveXenForoDirAndArgs(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if debug {
		// Only the first argument is echoed. The remainder routinely carries
		// inline code (-r) or tokens, and this line ends up in terminal
		// scrollback and CI logs.
		label := "Xdebug enabled"
		if len(phpArgs) > 0 {
			label += ": " + ui.Command.Render("php "+phpArgs[0])
		}

		ui.PrintInfo(label)

		if err := runner.ExecOrRun(ctx, "xf", map[string]string{"XDEBUG_SESSION": "1"}, os.Stdin, os.Stdout, os.Stderr, append([]string{"php"}, phpArgs...)...); err != nil {
			return passthroughError(err, "failed to run PHP")
		}

		return nil
	}

	if err := runner.ExecOrRun(ctx, "xf", nil, os.Stdin, os.Stdout, os.Stderr, append([]string{"php"}, phpArgs...)...); err != nil {
		return passthroughError(err, "failed to run PHP")
	}

	return nil
}
