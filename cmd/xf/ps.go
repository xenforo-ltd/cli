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

If no path is provided, the current directory will be searched for a XenForo installation.`,
	Example: `  # List containers in current directory
  xf ps

  # List containers in specific directory
  xf ps ./my-project`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: "env",
	RunE:    runPs,
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

	containers, err := runner.PSInfo(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		ui.PrintEmpty("No containers running")
		return nil
	}

	rows := make([][]string, 0, len(containers))
	for _, c := range containers {
		var state string

		switch c.State {
		case "running":
			state = ui.Success.Render("running")
		case "exited", "dead":
			state = ui.Error.Render(c.State)
		default:
			state = ui.Warning.Render(c.State)
		}

		rows = append(rows, []string{c.Service, c.Name, state, c.Status, c.Ports})
	}

	ui.PrintTable([]string{"SERVICE", "CONTAINER", "STATE", "STATUS", "PORTS"}, rows)

	return nil
}
