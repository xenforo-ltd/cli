package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/reboot"
)

// newRebootCommand builds the reboot command.
func newRebootCommand() *cobra.Command {
	return &cobra.Command{
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
}

// runReboot restarts the selected Docker environment.
func runReboot(cmd *cobra.Command, args []string) error {
	return reboot.Run(cmd.Context(), args)
}
