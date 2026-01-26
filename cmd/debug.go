package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"xf/internal/dockercompose"
	"xf/internal/ui"
)

var debugCmd = &cobra.Command{
	Use:   "debug <command> [args...]",
	Short: "Run XenForo CLI commands with XDebug",
	Long: `Run XenForo CLI commands with XDebug enabled for debugging.

This is the equivalent of running with XDEBUG_SESSION=1 to trigger your IDE debugger.

Examples:
  # Debug xf-dev:import
  xf debug xf-dev:import

  # Debug addon build with options
  xf debug xf-addon:build-release MyAddon

  # Debug any xf command
  xf debug cron:run
  xf debug user:create --username test`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDebug,
}

func init() {
	rootCmd.AddCommand(debugCmd)
}

func runDebug(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory")
	}

	xfDir, err := dockercompose.GetXenForoDir(cwd)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return err
	}

	ui.PrintInfo(fmt.Sprintf("Running with XDebug: %s", args[0]))

	return runner.XFCommandDebug(args...)
}
