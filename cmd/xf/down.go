package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var downCmd = &cobra.Command{
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

	ui.PrintInfo("Stopping Docker environment " + ui.Bold.Render(runner.Instance()))

	if err := runner.Compose(cmd.Context(), os.Stdin, os.Stdout, os.Stderr, downComposeArgs()...); err != nil {
		return fmt.Errorf("failed to stop Docker environment: %w", err)
	}

	ui.Println()
	ui.SuccessBox("Docker environment stopped", nil)

	return nil
}

// downComposeArgs lists the compose arguments for stopping an environment.
// It deliberately omits --volumes so the database and other volume data
// survive a later start, unlike Destroy.
func downComposeArgs() []string {
	return []string{"down"}
}
