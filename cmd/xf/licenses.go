package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/licenses"
)

var licensesCmd = &cobra.Command{
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

var flagLicensesJSON bool

func init() {
	licensesCmd.Flags().BoolVar(&flagLicensesJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(licensesCmd)
}

func runLicenses(cmd *cobra.Command, args []string) error {
	return licenses.Run(cmd.Context(), flagLicensesJSON)
}
