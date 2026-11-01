package main

import (
	"github.com/spf13/cobra"

	command "github.com/xenforo-ltd/cli/internal/commands/version"
)

var (
	flagVersionJSON  bool
	flagVersionShort bool
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long: `Display the version, build commit, build date, and runtime information.

Shows detailed build information including the exact Git commit and
build timestamp when available.`,
	Example: `  # Show full version info
  xf version

  # Show only version number
  xf version --short

  # Output as JSON (useful for scripting)
  xf version --json`,
	Args:    cobra.NoArgs,
	GroupID: "maint",
	RunE:    runVersion,
}

func init() {
	versionCmd.Flags().BoolVar(&flagVersionJSON, "json", false, "output as JSON")
	versionCmd.Flags().BoolVarP(&flagVersionShort, "short", "s", false, "print only the version number")
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) error {
	return command.Run(cmd.Context(), command.Options{JSON: flagVersionJSON, Short: flagVersionShort})
}
