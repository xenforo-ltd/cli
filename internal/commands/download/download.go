package download

import (
	"context"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// Options selects a license, download and version, and whether to bypass cache.
type Options struct {
	LicenseKey string
	ID         string
	VersionID  int
	Force      bool
}

// Run lists available downloads or versions, or downloads a selected version.
func Run(ctx context.Context, options Options) error {
	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	if options.ID == "" {
		return listDownloadables(ctx, client, options.LicenseKey)
	}

	if options.VersionID == 0 {
		return listVersions(ctx, client, options.LicenseKey, options.ID)
	}

	return performDownload(ctx, client, options.LicenseKey, options.ID, options.VersionID, options.Force)
}

// listDownloadables displays the packages available for a license.
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

	pairs := make([]ui.KVPair, 0, len(downloadables.Downloadables))
	for _, d := range downloadables.Downloadables {
		pairs = append(pairs, ui.KV(d.DownloadID, d.Title))
	}

	ui.PrintKeyValuePadded(pairs)

	ui.PrintHint("Add " + ui.Command.Render("--download <id>") + " to choose a package")

	return nil
}

// listVersions displays the available versions of a licensed package.
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

// performDownload retrieves a version into the cache, reusing a valid entry
// unless forced, and clears progress output before reporting an error.
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
		entry, err := cache.FindValid(cacheManager, licenseKey, downloadID, info.VersionString)
		if err != nil {
			return fmt.Errorf("failed to check cache for %s %s: %w", downloadID, info.VersionString, err)
		}

		if entry != nil {
			ui.SuccessBox("Already cached", []ui.KVPair{
				ui.KV("File", ui.Path.Render(ui.ShortHome(entry.FilePath))),
				ui.KV("Version", ui.Version.Render(info.VersionString)),
				ui.KV("Size", ui.FormatBytes(entry.Metadata.Size)),
			})

			ui.PrintHint("Add " + ui.Command.Render("--force") + " to re-download")

			return nil
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

	result, err := cacheManager.Download(ctx, opts, accessToken, progress)
	if err != nil {
		if progressBar != nil {
			progressBar.Abandon()
		}

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

	title := "Download complete"
	if result.WasCached {
		title = "Already cached"
	}

	ui.SuccessBox(title, []ui.KVPair{
		ui.KV("File", ui.Path.Render(ui.ShortHome(result.Entry.FilePath))),
		ui.KV("Version", ui.Version.Render(info.VersionString)),
		ui.KV("Size", ui.FormatBytes(result.Entry.Metadata.Size)),
	})

	return nil
}
