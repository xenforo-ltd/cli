package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/compose"
)

// newComposeCommand builds the compose passthrough command. Arguments and
// flags after its name belong to Docker Compose; xf flags must precede it.
func newComposeCommand() *cobra.Command {
	return &cobra.Command{
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
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		GroupID:            "run",
		RunE:               runCompose,
	}
}

// runCompose forwards arguments to Docker Compose.
func runCompose(cmd *cobra.Command, args []string) error {
	return compose.Run(cmd.Context(), args)
}
