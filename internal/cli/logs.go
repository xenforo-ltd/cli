package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/logs"
)

// flagLogsFollow selects continuous container log output.
var flagLogsFollow bool

// newLogsCommand builds the logs command.
func newLogsCommand() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs [path] [services...]",
		Short: "Show container logs",
		Long: `Show logs from Docker containers.

If no path is provided, the current directory will be searched for a XenForo installation.
Specific services can be specified, or all services will be shown.`,
		Example: `  # Show all logs
  xf logs

  # Follow logs (like tail -f)
  xf logs --follow

  # Show logs for specific services
  xf logs xf mysql

  # Show logs in specific directory
  xf logs ./my-project`,
		Args:    cobra.ArbitraryArgs,
		GroupID: "env",
		RunE:    runLogs,
	}

	logsCmd.Flags().BoolVarP(&flagLogsFollow, "follow", "f", false, "Follow log output")

	return logsCmd
}

// runLogs displays or follows logs for the selected services.
func runLogs(cmd *cobra.Command, args []string) error {
	return logs.Run(cmd.Context(), args, flagLogsFollow)
}
