package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var composerCmd = &cobra.Command{
	Use:   "composer [path] [args...]",
	Short: "Run Composer commands",
	Long: `Run Composer commands in the Docker environment.

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
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(0),
	GroupID:            "run",
	RunE:               runComposer,
}

func init() {
	rootCmd.AddCommand(composerCmd)
}

func runComposer(cmd *cobra.Command, args []string) error {
	xfDir, composerArgs, err := resolveXenForoDirAndArgs(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintInfo("Running: composer " + strings.Join(composerArgs, " "))

	if err := runner.Composer(cmd.Context(), composerArgs...); err != nil {
		return fmt.Errorf("composer command failed: %w", err)
	}

	return nil
}
