package downloads

import (
	"context"
	"fmt"
	"strings"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/extract"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// ProductTitles returns licensed product titles, falling back to the XenForo
// core title alone if the downloadable products cannot be retrieved.
func ProductTitles(ctx context.Context, client *customerapi.Client, licenseKey string) map[string]string {
	out := map[string]string{
		"xenforo": "XenForo",
	}

	downloadables, err := client.GetLicenseDownloadables(ctx, licenseKey)
	if err != nil {
		return out
	}

	for _, d := range downloadables.Downloadables {
		out[d.DownloadID] = d.Title
	}

	return out
}

// ProductTitle returns the display title for product, falling back to the
// product ID when the license does not report a title.
func ProductTitle(titles map[string]string, product string) string {
	if title := titles[product]; title != "" {
		return title
	}

	return product
}

// LicenseLabel formats the license key with its site title and URL, or its
// product title if neither site field is available.
func LicenseLabel(lic customerapi.License) string {
	var parts []string
	if lic.SiteTitle != "" {
		parts = append(parts, lic.SiteTitle)
	}

	if lic.SiteURL != "" {
		parts = append(parts, lic.SiteURL)
	}

	if len(parts) == 0 && lic.ProductTitle != "" {
		parts = append(parts, lic.ProductTitle)
	}

	if len(parts) == 0 {
		return lic.LicenseKey
	}

	return fmt.Sprintf("%s (%s)", lic.LicenseKey, strings.Join(parts, " - "))
}

// ExtractCachedFiles extracts cached archives into targetPath, processing core
// first and other products in unspecified order. It stops at the first error;
// files extracted earlier are not rolled back.
func ExtractCachedFiles(cachedFiles map[string]*cache.Entry, targetPath string, titleMap map[string]string, verb string) error {
	if entry, ok := cachedFiles["xenforo"]; ok {
		if err := extractEntry(entry, targetPath, "xenforo", "XenForo core", verb); err != nil {
			return err
		}
	}

	for product, entry := range cachedFiles {
		if product == "xenforo" {
			continue
		}

		title := ProductTitle(titleMap, product)

		if err := extractEntry(entry, targetPath, product, title, verb); err != nil {
			return err
		}
	}

	return nil
}

// extractEntry extracts an archive while reporting progress and completion.
func extractEntry(entry *cache.Entry, targetPath, product, title, verb string) error {
	startMessage := "Extracting " + title
	progressLabel := startMessage
	doneLabel := verb + " " + title
	errorName := product

	if product == "xenforo" {
		progressLabel = "Extracting files"
		errorName = "XenForo"
	}

	spinner := ui.NewSpinner(startMessage)
	spinner.Start()

	fileCount := 0
	progress := func(current, total int, filename string) {
		fileCount = current
		if total > 0 {
			spinner.UpdateMessage(fmt.Sprintf("%s (%d/%d)", progressLabel, current, total))
		}
	}

	if err := extract.XenForoZip(entry.FilePath, targetPath, progress); err != nil {
		spinner.Stop()

		return fmt.Errorf("failed to extract %s: %w", errorName, err)
	}

	spinner.StopWithMessage("success", fmt.Sprintf("%s (%s)", doneLabel, ui.Plural(fileCount, "file", "files")))

	return nil
}
