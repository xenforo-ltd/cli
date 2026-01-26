package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"xf/internal/dockercompose"
	"xf/internal/ui"
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
	var xfDir string
	var composerArgs []string

	if len(args) > 0 {
		potentialPath := args[0]
		if dir, err := getXenForoDir([]string{potentialPath}); err == nil {
			xfDir = dir
			composerArgs = args[1:]
		} else {
			xfDir, err = getXenForoDir(nil)
			if err != nil {
				return err
			}
			composerArgs = args
		}
	} else {
		var err error
		xfDir, err = getXenForoDir(nil)
		if err != nil {
			return err
		}
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return err
	}

	ui.PrintInfo(fmt.Sprintf("Running: composer %s", composerArgs))

	return runner.Composer(composerArgs...)
}
