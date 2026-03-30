package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var composerCmd = &cobra.Command{
	Use:   "composer [path] -- [args...]",
	Short: "Run Composer commands",
	Long: `Run Composer commands in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.
All arguments after -- are passed to Composer.

Examples:
  # Install dependencies
  xf composer -- install

  # Update dependencies
  xf composer -- update

  # Run composer in specific directory
  xf composer ./my-project -- install`,
	Args: cobra.MinimumNArgs(0),
	RunE: runComposer,
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
