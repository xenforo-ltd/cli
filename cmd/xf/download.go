package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/download"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download XenForo packages",
	Long: `Download XenForo packages to the local cache.

Downloads are cached locally and verified with checksums. Subsequent
downloads of the same version will use the cached copy.

The download command works in stages:
  1. Without --download: Lists available packages for the license
  2. With --download: Lists available versions for that package
  3. With --download and --version: Downloads the specified version`,
	Example: `  # List available packages for a license
  xf download --license XF123-ABCD-1234

  # List available versions for XenForo
  xf download --license XF123-ABCD-1234 --download xenforo

  # Download a specific version
  xf download --license XF123-ABCD-1234 --download xenforo --version 12345

  # Force re-download even if cached
  xf download --license XF123-ABCD-1234 --download xenforo --version 12345 --force`,
	Args:    cobra.NoArgs,
	GroupID: "start",
	RunE:    runDownload,
}

var (
	flagDownloadLicenseKey string
	flagDownloadID         string
	flagDownloadVersionID  int
	flagDownloadForce      bool
)

func init() {
	downloadCmd.Flags().StringVar(&flagDownloadLicenseKey, "license", "", "license key")
	downloadCmd.Flags().StringVar(&flagDownloadID, "download", "", "download ID (e.g., xenforo, xfmg)")
	downloadCmd.Flags().IntVar(&flagDownloadVersionID, "version", 0, "version ID to download")
	downloadCmd.Flags().BoolVar(&flagDownloadForce, "force", false, "force re-download even if cached")
	if err := downloadCmd.MarkFlagRequired("license"); err != nil {
		cobra.CheckErr(err)
	}
	rootCmd.AddCommand(downloadCmd)
}

func runDownload(cmd *cobra.Command, args []string) error {
	return download.Run(cmd.Context(), download.Options{LicenseKey: flagDownloadLicenseKey, ID: flagDownloadID, VersionID: flagDownloadVersionID, Force: flagDownloadForce})
}
