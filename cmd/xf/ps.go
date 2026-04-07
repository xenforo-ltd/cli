package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var psCmd = &cobra.Command{
	Use:   "ps [path]",
	Short: "List running containers",
	Long: `List the running Docker containers for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.

Examples:
  # List containers in current directory
  xf ps

  # List containers in specific directory
  xf ps ./my-project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPs,
}

func init() {
	rootCmd.AddCommand(psCmd)
}

func runPs(cmd *cobra.Command, args []string) error {
	xfDir, err := getXenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintInfo("Container status:")

	if err := runner.PS(cmd.Context()); err != nil {
		return fmt.Errorf("failed to list container status: %w", err)
	}

	return nil
}
