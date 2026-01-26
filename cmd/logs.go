package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"xf/internal/dockercompose"
	"xf/internal/ui"
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
	var xfDir string
	var services []string

	if len(args) > 0 {
		potentialPath := args[0]
		if dir, err := getXenForoDir([]string{potentialPath}); err == nil {
			xfDir = dir
			services = args[1:]
		} else {
			var err error
			xfDir, err = getXenForoDir(nil)
			if err != nil {
				return err
			}
			services = args
		}
	} else {
		var err error
		xfDir, err = getXenForoDir(nil)
		if err != nil {
			return err
		}
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return err
	}

	if len(services) > 0 {
		ui.PrintInfo(fmt.Sprintf("Showing logs for: %s", strings.Join(services, ", ")))
	} else {
		ui.PrintInfo("Showing logs for all services")
	}

	return runner.Logs(flagLogsFollow, services...)
}
