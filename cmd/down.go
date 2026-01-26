package cmd

import (
	"github.com/spf13/cobra"

	"xf/internal/dockercompose"
	"xf/internal/ui"
)

var downCmd = &cobra.Command{
	Use:   "down [path]",
	Short: "Stop the Docker environment",
	Long: `Stop and remove the Docker containers for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.

Examples:
  # Stop in current directory (auto-detect)
  xf down

  # Stop specific directory
  xf down ./my-project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	xfDir, err := getXenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return err
	}

	ui.PrintInfo("Stopping Docker environment...")

	if err := runner.Down(); err != nil {
		return err
	}

	ui.PrintSuccess("Docker environment stopped")
	return nil
}
