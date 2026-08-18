package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

var debugCmd = &cobra.Command{
	Use:   "debug <command> [args...]",
	Short: "Run XenForo CLI commands with Xdebug",
	Long: `Run XenForo CLI commands with Xdebug enabled for debugging.

This is the equivalent of running with XDEBUG_SESSION=1 to trigger your IDE debugger.`,
	Example: `  # Debug xf-dev:import
  xf debug xf-dev:import

  # Debug addon build with options
  xf debug xf-addon:build-release MyAddon

  # Debug any xf command
  xf debug cron:run
  xf debug user:create --username test`,
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(1),
	GroupID:            "run",
	RunE:               runDebug,
}

func init() {
	rootCmd.AddCommand(debugCmd)
}

func runDebug(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine the current directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return fmt.Errorf("failed to find XenForo directory: %w", err)
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintInfo("Running with Xdebug: " + args[0])

	if err := runner.XFCommandDebug(cmd.Context(), args...); err != nil {
		return fmt.Errorf("failed to run XenForo command with Xdebug: %w", err)
	}

	return nil
}
