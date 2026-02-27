package cmd

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
)

var composeCmd = &cobra.Command{
	Use:   "compose [path] -- [args...]",
	Short: "Run a Docker Compose command",
	Long: `Run a Docker Compose command directly.

If no path is provided, the current directory will be searched for a XenForo installation.
All arguments after -- are passed directly to 'docker compose'.

Examples:
  # List services
  xf compose -- ps

  # Build services
  xf compose -- build

  # Execute inside a running service
  xf compose -- exec xf mysql -u root`,
	Args: cobra.MinimumNArgs(0),
	RunE: runCompose,
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
		return err
	}

	return runner.Compose(composeArgs...)
}
