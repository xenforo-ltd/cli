package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
)

var composeCmd = &cobra.Command{
	Use:   "compose [path] [args...]",
	Short: "Run a Docker Compose command",
	Long: `Run a Docker Compose command directly.

If no path is provided, the current directory will be searched for a XenForo installation.

Everything after 'compose' is passed to 'docker compose', including flags. Give
xf's own flags before the command name, and use 'xf help compose' for this help.

Examples:
  # List services
  xf compose ps

  # Build services
  xf compose build

  # Execute inside a running service
  xf compose exec xf mysql -u root`,
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(0),
	RunE:               runCompose,
}

func init() {
	rootCmd.AddCommand(composeCmd)
}

func runCompose(cmd *cobra.Command, args []string) error {
	xfDir, composeArgs, err := resolveXenForoDirAndArgs(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if err := runner.Compose(cmd.Context(), composeArgs...); err != nil {
		return fmt.Errorf("docker compose command failed: %w", err)
	}

	return nil
}
