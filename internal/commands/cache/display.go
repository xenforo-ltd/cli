package cache

import (
	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// printCacheHeader prints the shared summary line used by both the table and
// verbose cache list views.
func printCacheHeader(entries []*cache.Entry, totalSize int64) {
	ui.PrintInfo(ui.Plural(len(entries), "cached download", "cached downloads") + " (" + ui.FormatBytes(totalSize) + ")")
	ui.Println()
}

func runCacheListTable(entries []*cache.Entry, totalSize int64) error {
	printCacheHeader(entries, totalSize)

	headers := []string{"LICENSE", "PRODUCT", "VERSION", "SIZE", "DOWNLOADED"}
	rows := make([][]string, 0, len(entries))

	for _, e := range entries {
		rows = append(rows, []string{
			e.LicenseKey,
			e.Metadata.DownloadID,
			ui.Version.Render("v" + e.Metadata.Version),
			ui.FormatBytes(e.Metadata.Size),
			ui.FormatDate(e.Metadata.DownloadedAt),
		})
	}

	ui.Println(ui.NewTable(headers, rows))
	ui.Println()
	ui.PrintHint("Run " + ui.Command.Render("xf cache list -v") + " for detailed information")

	return nil
}

func runCacheListVerbose(entries []*cache.Entry, totalSize int64) error {
	printCacheHeader(entries, totalSize)

	currentLicense := ""
	for _, e := range entries {
		if e.LicenseKey != currentLicense {
			if currentLicense != "" {
				ui.Println()
			}

			ui.Printf("%s %s\n", ui.Dim.Render(ui.SymbolBullet), ui.Bold.Render("License "+e.LicenseKey))
			currentLicense = e.LicenseKey
		}

		ui.Printf("%s%s %s\n", ui.Indent1, ui.Bold.Render(e.Metadata.DownloadID), ui.Version.Render("v"+e.Metadata.Version))

		checksum := ui.Dim.Render("—")
		if e.Metadata.Checksum != "" {
			checksum = ui.Dim.Render(e.Metadata.Checksum)
		}

		pairs := []ui.KVPair{
			ui.KV("File", e.Metadata.Filename),
			ui.KV("Size", ui.FormatBytes(e.Metadata.Size)),
			ui.KV("Downloaded", ui.FormatDateTime(e.Metadata.DownloadedAt)),
			ui.KV("Checksum", checksum),
		}

		ui.PrintKeyValuePaddedWithIndent(pairs, ui.Indent2)
	}

	return nil
}
