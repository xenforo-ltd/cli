package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/reboot"
)

var rebootCmd = &cobra.Command{
	Use:   "reboot [path]",
	Short: "Restart the Docker environment",
	Long: `Stop and restart the Docker containers for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.`,
	Example: `  # Reboot in current directory (auto-detect)
  xf reboot

  # Reboot specific directory
  xf reboot ./my-project`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: "env",
	RunE:    runReboot,
}

func init() {
	rootCmd.AddCommand(rebootCmd)
}

func runReboot(cmd *cobra.Command, args []string) error {
	return reboot.Run(cmd.Context(), args)
}
