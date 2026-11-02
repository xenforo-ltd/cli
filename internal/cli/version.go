package cli

import (
	"github.com/spf13/cobra"

	versioncommand "github.com/xenforo-ltd/cli/internal/commands/version"
)

var (
	// flagVersionJSON selects JSON version output.
	flagVersionJSON bool
	// flagVersionShort selects version-number-only output.
	flagVersionShort bool
)

// newVersionCommand builds the version command.
func newVersionCommand() *cobra.Command {
	versionCmd := &cobra.Command{
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

	versionCmd.Flags().BoolVar(&flagVersionJSON, "json", false, "output as JSON")
	versionCmd.Flags().BoolVarP(&flagVersionShort, "short", "s", false, "print only the version number")

	return versionCmd
}

// runVersion prints version information in the selected format.
func runVersion(cmd *cobra.Command, args []string) error {
	return versioncommand.Run(cmd.Context(), versioncommand.Options{JSON: flagVersionJSON, Short: flagVersionShort})
}
