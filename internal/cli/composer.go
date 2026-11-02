package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/composer"
)

// newComposerCommand builds the composer passthrough command. Arguments and
// flags after its name belong to Composer; xf flags must precede it.
func newComposerCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "composer [path] [args...]",
		Short: "Run Composer commands",
		Long: `Run Composer commands in the Docker environment when configured,
or on the host otherwise.

If no path is provided, the current directory will be searched for a XenForo installation.

Everything after 'composer' is passed to Composer, including flags. Give xf's own
flags before the command name, and use 'xf help composer' for this help text.`,
		Example: `  # Install dependencies
  xf composer install

  # Update dependencies
  xf composer update

  # Pass flags directly
  xf composer outdated --direct

  # Run composer in specific directory
  xf composer ./my-project install`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		GroupID:            "run",
		RunE:               runComposer,
	}
}

// runComposer forwards arguments to Composer.
func runComposer(cmd *cobra.Command, args []string) error {
	return composer.Run(cmd.Context(), args)
}
