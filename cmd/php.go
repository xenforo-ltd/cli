package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var phpCmd = &cobra.Command{
	Use:   "php [path] -- [args...]",
	Short: "Run PHP commands",
	Long: `Run PHP in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.
All arguments after -- are passed to PHP.

Examples:
  # Check PHP version
  xf php -- -v

  # Run a PHP script
  xf php -- my-script.php

  # Run PHP in specific directory
  xf php ./my-project -- -v`,
	Args: cobra.MinimumNArgs(0),
	RunE: runPHP,
}

var phpDebugCmd = &cobra.Command{
	Use:   "php-debug [path] -- [args...]",
	Short: "Run PHP with XDebug",
	Long: `Run PHP with XDebug enabled in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.
All arguments after -- are passed to PHP.

Examples:
  # Run PHP script with XDebug
  xf php-debug -- my-script.php`,
	Args: cobra.MinimumNArgs(0),
	RunE: runPHPDebug,
}

func init() {
	rootCmd.AddCommand(phpCmd)
	rootCmd.AddCommand(phpDebugCmd)
}

func runPHP(cmd *cobra.Command, args []string) error {
	return runPHPWithMode(cmd.Context(), args, false)
}

func runPHPDebug(cmd *cobra.Command, args []string) error {
	return runPHPWithMode(cmd.Context(), args, true)
}

func runPHPWithMode(ctx context.Context, args []string, debug bool) error {
	xfDir, phpArgs, err := resolveXenForoDirAndArgs(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if debug {
		ui.PrintInfo("Running with XDebug: php " + strings.Join(phpArgs, " "))

		if err := runner.PHPDebug(ctx, phpArgs...); err != nil {
			return fmt.Errorf("failed to run PHP with XDebug: %w", err)
		}

		return nil
	}

	if err := runner.PHP(ctx, phpArgs...); err != nil {
		return fmt.Errorf("failed to run PHP command: %w", err)
	}

	return nil
}
