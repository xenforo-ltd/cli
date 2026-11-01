package setup

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func CheckPrerequisites(ctx context.Context) error {
	ui.Println(ui.Bold.Render("Checking prerequisites..."))
	if err := environment.CheckDockerRunning(ctx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, exec.ErrNotFound) {
			return clierror.WithHint(clierror.MarkAs(err, "Docker is not installed or not in your PATH"), "Install Docker and try again")
		}
		return clierror.WithHint(clierror.MarkAs(err, "Docker does not appear to be running"), "Start Docker Desktop and try again")
	}
	ui.PrintSuccess("Docker is running")
	if err := environment.CheckDockerComposeAvailable(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker Compose is available: %w", err)
	}
	ui.PrintSuccess("Docker Compose is available")
	ui.Println()
	return nil
}
