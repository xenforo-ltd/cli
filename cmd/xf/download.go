package main

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
	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
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
	spinner := ui.NewSpinner("Fetching downloads for " + licenseKey)
	spinner.Start()

	downloadables, err := client.GetLicenseDownloadables(ctx, licenseKey)

	spinner.Stop()

	if err != nil {
		return fmt.Errorf("failed to fetch available downloads for license %s: %w", licenseKey, err)
	}

	if len(downloadables.Downloadables) == 0 {
		ui.PrintEmpty("No downloadables available for this license")
		return nil
	}

	ui.Println(ui.Bold.Render("Available downloads"))
	ui.Println()

	for _, d := range downloadables.Downloadables {
		ui.PrintKeyValuePadded([]ui.KVPair{ui.KV(d.DownloadID, d.Title)})
	}

	ui.PrintHint("Add " + ui.Command.Render("--download <id>") + " to choose a package")

	return nil
}

func listVersions(ctx context.Context, client *customerapi.Client, licenseKey string, downloadID string) error {
	spinner := ui.NewSpinner("Fetching versions for " + downloadID)
	spinner.Start()

	versions, err := client.GetLicenseVersions(ctx, licenseKey, downloadID)

	spinner.Stop()

	if err != nil {
		return fmt.Errorf("failed to fetch versions for %s: %w", downloadID, err)
	}

	if len(versions.Versions) == 0 {
		ui.PrintEmpty("No versions available for this download")
		return nil
	}

	ui.Println(ui.Bold.Render("Available versions"))
	ui.Println()

	for _, v := range versions.Versions {
		stable := ""
		if v.Stable {
			stable = ui.Dim.Render(" (stable)")
		}

		ui.Printf("%s%s %s\n", ui.Indent1, ui.Version.Render(v.VersionStr)+stable, ui.Muted.Render(fmt.Sprintf("(id %d)", v.VersionID)))
	}

	ui.PrintHint("Add " + ui.Command.Render("--version <id>") + " to choose a version")

	return nil
}

func performDownload(ctx context.Context, client *customerapi.Client, licenseKey string, downloadID string, versionID int, force bool) error {
	spinner := ui.NewSpinner("Fetching download info for " + downloadID)
	spinner.Start()

	info, err := client.GetDownloadInfo(ctx, licenseKey, downloadID, versionID)

	spinner.Stop()

	if err != nil {
		return fmt.Errorf("failed to get download info for %s version %d: %w", downloadID, versionID, err)
	}

	cacheManager, err := cache.NewManager()
	if err != nil {
		return fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	if !force {
		entry, err := cacheManager.GetEntry(licenseKey, downloadID, info.VersionString)
		if err != nil && !errors.Is(err, cache.ErrCacheMiss) {
			return fmt.Errorf("failed to check cache for %s %s: %w", downloadID, info.VersionString, err)
		}

		if entry != nil {
			valid, err := cacheManager.Verify(entry)
			if err == nil && valid {
				if _, statErr := os.Stat(entry.FilePath); statErr == nil {
					ui.SuccessBox("Already cached", []ui.KVPair{
						ui.KV("File", ui.Path.Render(ui.ShortHome(entry.FilePath))),
						ui.KV("Version", ui.Version.Render(info.VersionString)),
						ui.KV("Size", ui.FormatBytes(entry.Metadata.Size)),
					})
					ui.PrintHint("Add " + ui.Command.Render("--force") + " to re-download")

					return nil
				}
			}
		}
	}

	accessToken, err := client.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token for download: %w", err)
	}

	downloadURL := client.GetDownloadURL(licenseKey, downloadID, versionID)

	var (
		progressBar *ui.ProgressBar
		dlSpinner   *ui.Spinner
	)

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
			if dlSpinner != nil {
				dlSpinner.Stop()
				dlSpinner = nil
			}

			if progressBar == nil {
				progressBar = ui.NewProgressBar(total, info.Filename)
			}

			progressBar.Update(current)
		} else if dlSpinner == nil {
			dlSpinner = ui.NewSpinner("Downloading " + info.Filename)
			dlSpinner.Start()
		}
	}

	result, err := cacheManager.DownloadWithAuth(ctx, opts, accessToken, progress)
	if err != nil {
		if dlSpinner != nil {
			dlSpinner.Stop()
		}

		return fmt.Errorf("failed to download %s version %d: %w", downloadID, versionID, err)
	}

	if progressBar != nil {
		progressBar.Finish()
	}

	if dlSpinner != nil {
		dlSpinner.Stop()
	}

	if result.WasCached {
		ui.SuccessBox("Already cached", []ui.KVPair{
			ui.KV("File", ui.Path.Render(ui.ShortHome(result.Entry.FilePath))),
			ui.KV("Version", ui.Version.Render(info.VersionString)),
			ui.KV("Size", ui.FormatBytes(result.Entry.Metadata.Size)),
		})
	} else {
		ui.SuccessBox("Download complete", []ui.KVPair{
			ui.KV("File", ui.Path.Render(ui.ShortHome(result.Entry.FilePath))),
			ui.KV("Version", ui.Version.Render(info.VersionString)),
			ui.KV("Size", ui.FormatBytes(result.Entry.Metadata.Size)),
		})
	}

	return nil
}
