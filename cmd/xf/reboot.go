package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
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
	xfDir, err := getXenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ctx := cmd.Context()

	ui.PrintStep(1, 2, "Stopping "+runner.Instance())

	if err := runner.Down(ctx); err != nil {
		return fmt.Errorf("failed to stop Docker environment: %w", err)
	}

	ui.PrintStep(2, 2, "Starting "+runner.Instance())

	if err := runner.Up(ctx, true); err != nil {
		return fmt.Errorf("failed to start Docker environment: %w", err)
	}

	details := []ui.KVPair{}
	if url, err := runner.GetURL(ctx); err == nil {
		details = append(details, ui.KV("URL", ui.URL.Render(url)))
	} else {
		ui.PrintWarning("Could not determine the site URL")
	}
	ui.Println()
	ui.SuccessBox("Docker environment restarted", details)

	return nil
}
