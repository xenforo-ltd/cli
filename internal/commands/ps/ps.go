package ps

import (
	"context"
	"fmt"

	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func Run(ctx context.Context, args []string) error {
	xfDir, err := passthrough.XenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	containers, err := runner.PSInfo(ctx)
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
