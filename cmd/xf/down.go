package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
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
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintInfo("Stopping Docker environment...")

	if err := runner.Down(cmd.Context()); err != nil {
		return fmt.Errorf("failed to stop Docker environment: %w", err)
	}

	ui.PrintSuccess("Docker environment stopped")

	return nil
}
