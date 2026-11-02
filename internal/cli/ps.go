package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/ps"
)

// newPSCommand builds the ps command.
func newPSCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ps [path]",
		Short: "List running containers",
		Long: `List the running Docker containers for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.`,
		Example: `  # List containers in current directory
  xf ps

  # List containers in specific directory
  xf ps ./my-project`,
		Args:    cobra.MaximumNArgs(1),
		GroupID: "env",
		RunE:    runPs,
	}
}

func runPs(cmd *cobra.Command, args []string) error {
	return ps.Run(cmd.Context(), args)
}
