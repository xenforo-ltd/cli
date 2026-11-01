package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/up"
)

var upCmd = &cobra.Command{
	Use:   "up [path]",
	Short: "Start the Docker environment",
	Long: `Start the Docker environment for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.`,
	Example: `  # Start in current directory (auto-detect)
  xf up

  # Start specific directory
  xf up ./my-project

  # Start in foreground (not detached)
  xf up --no-detach`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: "env",
	RunE:    runUp,
}

var flagUpDetach bool

func init() {
	upCmd.Flags().BoolVar(&flagUpDetach, "detach", true, "Run containers in the background")
	upCmd.Flags().Bool("no-detach", false, "Run containers in the foreground")

	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	detach := flagUpDetach
	if cmd.Flags().Changed("no-detach") {
		detach = false
	}

	return up.Run(cmd.Context(), args, detach)
}
