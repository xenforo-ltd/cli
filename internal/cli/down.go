package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/down"
)

// newDownCommand builds the down command.
func newDownCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "down [path]",
		Short: "Stop the Docker environment",
		Long: `Stop and remove the Docker containers for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.`,
		Example: `  # Stop in current directory (auto-detect)
  xf down

  # Stop specific directory
  xf down ./my-project`,
		Args:    cobra.MaximumNArgs(1),
		GroupID: "env",
		RunE:    runDown,
	}
}

// runDown stops and removes the environment's containers.
func runDown(cmd *cobra.Command, args []string) error {
	return down.Run(cmd.Context(), args)
}
