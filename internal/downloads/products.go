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

func ExtractCachedFiles(cachedFiles map[string]*cache.Entry, targetPath string, titleMap map[string]string, verb string) error {
	if entry, ok := cachedFiles["xenforo"]; ok {
		spinner := ui.NewSpinner("Extracting XenForo core")
		spinner.Start()

		fileCount := 0
		progress := func(current, total int, filename string) {
			fileCount = current
			if total > 0 {
				spinner.UpdateMessage(fmt.Sprintf("Extracting files (%d/%d)", current, total))
			}
		}

		if err := extract.XenForoZip(entry.FilePath, targetPath, progress); err != nil {
			spinner.Stop()

			return fmt.Errorf("failed to extract XenForo: %w", err)
		}

		spinner.StopWithMessage("success", fmt.Sprintf("%s XenForo core (%s)", verb, ui.Plural(fileCount, "file", "files")))
	}

	for product, entry := range cachedFiles {
		if product == "xenforo" {
			continue
		}

		productName := product
		if titleMap != nil {
			if name := titleMap[product]; name != "" {
				productName = name
			}
		}

		spinner := ui.NewSpinner("Extracting " + productName)
		spinner.Start()

		fileCount := 0
		progress := func(current, total int, filename string) {
			fileCount = current
			if total > 0 {
				spinner.UpdateMessage(fmt.Sprintf("Extracting %s (%d/%d)", productName, current, total))
			}
		}

		if err := extract.XenForoZip(entry.FilePath, targetPath, progress); err != nil {
			spinner.Stop()

			return fmt.Errorf("failed to extract %s: %w", product, err)
		}

		spinner.StopWithMessage("success", fmt.Sprintf("%s %s (%s)", verb, productName, ui.Plural(fileCount, "file", "files")))
	}

	return nil
}
