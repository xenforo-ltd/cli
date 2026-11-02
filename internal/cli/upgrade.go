package cli

import (
	"github.com/spf13/cobra"
	"github.com/xenforo-ltd/cli/internal/commands/upgrade"
	"github.com/xenforo-ltd/cli/internal/completion"
)

var (
	flagUpgradeLicense string
	flagUpgradeVersion int
	flagUpgradeSkipCmd bool
)

// newUpgradeCommand builds the upgrade command.
func newUpgradeCommand() *cobra.Command {
	upgradeCmd := &cobra.Command{
		Use:   "upgrade [path]",
		Short: "Upgrade an existing XenForo installation",
		Long: `Upgrade an existing XenForo installation to a newer version.

This command:
  1. Detects the current XenForo version
  2. Shows available upgrade versions
  3. Downloads the new version
  4. Overlays new files (preserving config and data)
  5. Runs 'xf xf:upgrade' to complete the upgrade

The target directory must contain an existing XenForo installation.
If the installation was created with 'xf init', the license
key will be detected automatically. Otherwise, use --license to specify it.`,
		Example: `  # Interactive upgrade (prompts for version)
  xf upgrade ./my-project

  # Upgrade to a specific version
  xf upgrade ./my-project --version 2030971

  # Non-interactive upgrade
  xf upgrade ./my-project --version 2030971 --non-interactive`,
		Args: cobra.MaximumNArgs(1), GroupID: "env", RunE: runUpgrade,
	}

	upgradeCmd.Flags().StringVar(&flagUpgradeLicense, "license", "", "license key (auto-detected if not specified)")
	upgradeCmd.Flags().IntVar(&flagUpgradeVersion, "version", 0, "target version ID")
	upgradeCmd.Flags().BoolVar(&flagUpgradeSkipCmd, "skip-upgrade", false, "skip running xf:upgrade command")
	completion.Register(upgradeCmd, "license", completion.LicenseKeys)

	return upgradeCmd
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	return upgrade.Run(cmd.Context(), &upgrade.UpgradeOptions{
		TargetPath: targetPath, LicenseKey: flagUpgradeLicense,
		TargetVersionID: flagUpgradeVersion, SkipUpgrade: flagUpgradeSkipCmd,
	})
}
