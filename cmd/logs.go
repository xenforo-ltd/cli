package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var logsCmd = &cobra.Command{
	Use:   "logs [path] [services...]",
	Short: "Show container logs",
	Long: `Show logs from Docker containers.

If no path is provided, the current directory will be searched for a XenForo installation.
Specific services can be specified, or all services will be shown.

Examples:
  # Show all logs
  xf logs

  # Follow logs (like tail -f)
  xf logs --follow

  # Show logs for specific services
  xf logs xf mysql

  # Show logs in specific directory
  xf logs ./my-project`,
	Args: cobra.MinimumNArgs(0),
	RunE: runLogs,
}

var flagLogsFollow bool

func init() {
	logsCmd.Flags().BoolVarP(&flagLogsFollow, "follow", "f", false, "Follow log output")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	xfDir, services, err := resolveXenForoDirAndArgs(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if len(services) > 0 {
		ui.PrintInfo("Showing logs for: " + strings.Join(services, ", "))
	} else {
		ui.PrintInfo("Showing logs for all services")
	}

	if err := runner.Logs(cmd.Context(), flagLogsFollow, services...); err != nil {
		return fmt.Errorf("failed to show container logs: %w", err)
	}

	return nil
}
