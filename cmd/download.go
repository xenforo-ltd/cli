package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/ui"
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
  3. With --download and --version: Downloads the specified version

Examples:
  # List available packages for a license
  xf download --license XF123-ABCD-1234

  # List available versions for XenForo
  xf download --license XF123-ABCD-1234 --download xenforo

  # Download a specific version
  xf download --license XF123-ABCD-1234 --download xenforo --version 12345

  # Force re-download even if cached
  xf download --license XF123-ABCD-1234 --download xenforo --version 12345 --force`,
	RunE: runDownload,
}

var (
	flagDownloadLicenseKey string
	flagDownloadID         string
	flagDownloadVersionID  int
	flagDownloadForce      bool
)

func init() {
	downloadCmd.Flags().StringVar(&flagDownloadLicenseKey, "license", "", "license key (required)")
	downloadCmd.Flags().StringVar(&flagDownloadID, "download", "", "download ID (e.g., xenforo, xfmg)")
	downloadCmd.Flags().IntVar(&flagDownloadVersionID, "version", 0, "version ID to download")
	downloadCmd.Flags().BoolVar(&flagDownloadForce, "force", false, "force re-download even if cached")

	if err := downloadCmd.MarkFlagRequired("license"); err != nil {
		cobra.CheckErr(err)
	}

	rootCmd.AddCommand(downloadCmd)
}

func runDownload(cmd *cobra.Command, args []string) error {
	client, err := customerapi.NewClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
	defer cancel()

	if flagDownloadID == "" {
		return listDownloadables(ctx, client, flagDownloadLicenseKey)
	}

	if flagDownloadVersionID == 0 {
		return listVersions(ctx, client, flagDownloadLicenseKey, flagDownloadID)
	}

	return performDownload(ctx, client, flagDownloadLicenseKey, flagDownloadID, flagDownloadVersionID, flagDownloadForce)
}

func listDownloadables(ctx context.Context, client *customerapi.Client, licenseKey string) error {
	ui.PrintInfo(fmt.Sprintf("Fetching available downloads for license %s...", ui.Bold.Render(licenseKey)))
	fmt.Println()

	downloadables, err := client.GetLicenseDownloadables(ctx, licenseKey)
	if err != nil {
		return err
	}

	if len(downloadables.Downloadables) == 0 {
		ui.PrintWarning("No downloadables available for this license.")
		return nil
	}

	ui.Printf("%s Available downloads:\n\n", ui.StatusIcon("success"))

	for _, d := range downloadables.Downloadables {
		ui.Printf("%s%s %s\n", ui.Indent1, ui.Bold.Render(d.DownloadID), ui.Dim.Render("- "+d.Title))
	}

	ui.Printf("\nUse %s to specify which package to download.\n", ui.Command.Render("--download <id>"))

	return nil
}

func listVersions(ctx context.Context, client *customerapi.Client, licenseKey string, downloadID string) error {
	ui.PrintInfo(fmt.Sprintf("Fetching available versions for %s...", ui.Bold.Render(downloadID)))
	fmt.Println()

	versions, err := client.GetLicenseVersions(ctx, licenseKey, downloadID)
	if err != nil {
		return err
	}

	if len(versions.Versions) == 0 {
		ui.PrintWarning("No versions available for this download.")
		return nil
	}

	ui.Printf("%s Available versions:\n\n", ui.StatusIcon("success"))

	for _, v := range versions.Versions {
		stable := ""
		if v.Stable {
			stable = ui.Success.Render(" (stable)")
		}

		ui.Printf("%s%s %s%s\n", ui.Indent1, ui.Dim.Render(fmt.Sprintf("%d", v.VersionID)), ui.Version.Render(v.VersionStr), stable)
	}

	ui.Printf("\nUse %s to specify which version to download.\n", ui.Command.Render("--version <id>"))

	return nil
}

func performDownload(ctx context.Context, client *customerapi.Client, licenseKey string, downloadID string, versionID int, force bool) error {
	ui.PrintInfo(fmt.Sprintf("Getting download info for %s version %d...", ui.Bold.Render(downloadID), versionID))

	info, err := client.GetDownloadInfo(ctx, licenseKey, downloadID, versionID)
	if err != nil {
		return err
	}

	fmt.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("Filename", info.Filename),
		ui.KV("Version", ui.Version.Render(info.VersionString)),
	})

	cacheManager, err := cache.NewManager()
	if err != nil {
		return err
	}

	if !force {
		entry, err := cacheManager.GetEntry(licenseKey, downloadID, info.VersionString)
		if err != nil && !errors.Is(err, cache.ErrCacheMiss) {
			return err
		}

		if entry != nil {
			valid, err := cacheManager.Verify(entry)
			if err == nil && valid {
				if _, statErr := os.Stat(entry.FilePath); statErr == nil {
					fmt.Println()
					ui.PrintSuccess(fmt.Sprintf("Already cached: %s", ui.Path.Render(entry.FilePath)))
					fmt.Println()
					ui.PrintKeyValuePadded([]ui.KVPair{
						ui.KV("Size", ui.FormatBytes(entry.Metadata.Size)),
						ui.KV("Downloaded", entry.Metadata.DownloadedAt.Format("2006-01-02 15:04:05")),
					})
					ui.Printf("\nUse %s to re-download.\n", ui.Command.Render("--force"))

					return nil
				}
			}
		}
	}

	accessToken, err := client.GetAccessToken()
	if err != nil {
		return err
	}

	downloadURL := client.GetDownloadURL(licenseKey, downloadID, versionID)

	fmt.Println()
	ui.PrintInfo("Downloading...")

	var progressBar *ui.ProgressBar

	opts := cache.DownloadOptions{
		LicenseKey:     licenseKey,
		DownloadID:     downloadID,
		Version:        info.VersionString,
		URL:            downloadURL,
		Filename:       info.Filename,
		SkipCacheCheck: force,
	}

	progress := func(current, total int64) {
		if total > 0 {
			if progressBar == nil {
				progressBar = ui.NewProgressBar(total, "")
			}

			progressBar.Update(current)
		}
	}

	result, err := cacheManager.DownloadWithAuth(ctx, opts, accessToken, progress)
	if err != nil {
		return err
	}

	if progressBar != nil {
		progressBar.Finish()
	}

	fmt.Println()

	if result.WasCached {
		ui.PrintSuccess(fmt.Sprintf("Used cached file: %s", ui.Path.Render(result.Entry.FilePath)))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Downloaded: %s", ui.Path.Render(result.Entry.FilePath)))
		fmt.Println()
		ui.PrintKeyValuePadded([]ui.KVPair{
			ui.KV("Size", ui.FormatBytes(result.Entry.Metadata.Size)),
			ui.KV("Checksum", ui.Dim.Render(result.Entry.Metadata.Checksum[:16]+"...")),
		})
	}

	return nil
}
