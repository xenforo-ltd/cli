package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/compose"
)

var composeCmd = &cobra.Command{
	Use:   "compose [path] [args...]",
	Short: "Run a Docker Compose command",
	Long: `Run a Docker Compose command directly.

If no path is provided, the current directory will be searched for a XenForo installation.

Everything after 'compose' is passed to 'docker compose', including flags. Give
xf's own flags before the command name, and use 'xf help compose' for this help.`,
	Example: `  # List services
  xf compose ps

  # Build services
  xf compose build

  # Execute inside a running service
  xf compose exec xf mysql -u root`,
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	GroupID:            "run",
	RunE:               runCompose,
}

func init() {
	rootCmd.AddCommand(composeCmd)
}

func runCompose(cmd *cobra.Command, args []string) error {
	return compose.Run(cmd.Context(), args)
}
