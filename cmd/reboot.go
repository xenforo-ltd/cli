package cmd

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var rebootCmd = &cobra.Command{
	Use:   "reboot [path]",
	Short: "Restart the Docker environment",
	Long: `Stop and restart the Docker containers for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.

Examples:
  # Reboot in current directory (auto-detect)
  xf reboot

  # Reboot specific directory
  xf reboot ./my-project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReboot,
}

func init() {
	rootCmd.AddCommand(rebootCmd)
}

func runReboot(cmd *cobra.Command, args []string) error {
	xfDir, err := getXenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return err
	}

	ui.PrintInfo("Stopping Docker environment...")

	ctx := cmd.Context()

	if err := runner.Down(ctx); err != nil {
		return err
	}

	ui.PrintInfo("Starting Docker environment...")

	if err := runner.Up(ctx, true); err != nil {
		return err
	}

	ui.PrintSuccess("Docker environment restarted")

	return nil
}
