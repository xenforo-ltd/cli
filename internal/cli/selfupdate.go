package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/selfupdate"
)

// selfUpdateCheckOnly selects an update check without installation.
var selfUpdateCheckOnly bool

// newSelfUpdateCommand builds the self-update command.
func newSelfUpdateCommand() *cobra.Command {
	selfUpdateCmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update XenForo CLI to the latest version",
		Long: `Check for and install updates to the xf tool.

By default, this command will check for updates and install them automatically.
Use --check-only to just check if an update is available without installing.`,
		Example: `  # Check for and install updates
  xf self-update

  # Just check for updates without installing
  xf self-update --check-only`,
		Args:    cobra.NoArgs,
		GroupID: "maint",
		RunE:    runSelfUpdate,
	}

	selfUpdateCmd.Flags().BoolVar(&selfUpdateCheckOnly, "check-only", false,
		"only check for updates, don't install")

	return selfUpdateCmd
}

// runSelfUpdate checks for and optionally installs a CLI update.
func runSelfUpdate(cmd *cobra.Command, args []string) error {
	return selfupdate.Run(cmd.Context(), selfUpdateCheckOnly)
}
