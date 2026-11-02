package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/licenses"
)

var flagLicensesJSON bool

// newLicensesCommand builds the licenses command.
func newLicensesCommand() *cobra.Command {
	licensesCmd := &cobra.Command{
		Use:   "licenses",
		Short: "List your XenForo licenses",
		Long: `Display all XenForo licenses associated with your customer account.

Shows license details including product, status, expiration date, site URL,
and available extras (add-ons).`,
		Example: `  # List all licenses (compact table)
  xf licenses

  # List with full details
  xf licenses -v

  # Output as JSON (useful for scripting)
  xf licenses --json`,
		Args:    cobra.NoArgs,
		GroupID: "start",
		RunE:    runLicenses,
	}

	licensesCmd.Flags().BoolVar(&flagLicensesJSON, "json", false, "output as JSON")

	return licensesCmd
}

func runLicenses(cmd *cobra.Command, args []string) error {
	return licenses.Run(cmd.Context(), flagLicensesJSON)
}
