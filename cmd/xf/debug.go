package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/debug"
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
	return debug.Run(cmd.Context(), args)
}
