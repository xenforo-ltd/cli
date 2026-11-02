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
		rows = append(rows, []string{c.Service, c.Name, renderState(c.State), c.Status, c.Ports})
	}

	ui.Println(ui.NewTable([]string{"SERVICE", "CONTAINER", "STATE", "STATUS", "PORTS"}, rows))

	return nil
}

func renderState(state string) string {
	switch state {
	case "running":
		return ui.Success.Render(state)
	case "exited", "dead":
		return ui.Error.Render(state)
	default:
		return ui.Warning.Render(state)
	}
}
