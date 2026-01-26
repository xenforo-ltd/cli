package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"xf/internal/cache"
	"xf/internal/errors"
	"xf/internal/ui"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage cached downloads",
	Long: `Manage cached XenForo package downloads.

Downloaded packages are cached locally to avoid re-downloading. Use these
commands to view, manage, and clear the cache.

Examples:
  # List all cached downloads
  xf cache list

  # List cached downloads for a specific license
  xf cache list --license XF123-ABCD-1234

  # Show the cache directory path
  xf cache path

  # Clear all cached downloads
  xf cache purge --all`,
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached downloads",
	Long: `Display all cached downloads with their metadata.

Shows download information including version, file size, checksum,
and download date. Results can be filtered by license key.

Examples:
  # List all cached downloads (compact table)
  xf cache list

  # List with full details
  xf cache list -v

  # List in JSON format
  xf cache list --json

  # List only downloads for a specific license
  xf cache list --license XF123-ABCD-1234`,
	RunE: runCacheList,
}

var cachePurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove cached downloads",
	Long: `Remove cached downloads to free up disk space.

By default, requires explicit confirmation with --all flag. Use --license
to selectively remove downloads for a specific license.

Examples:
  # Remove all cached downloads
  xf cache purge --all

  # Remove downloads for a specific license only
  xf cache purge --license XF123-ABCD-1234`,
	RunE: runCachePurge,
}

var cachePathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show cache directory location",
	Long: `Display the path to the cache directory.

Useful for scripting or manually inspecting cached files.

Examples:
  # Show cache path
  xf cache path

  # Open cache directory in file manager (macOS)
  open $(xf cache path)`,
	RunE: runCachePath,
}

var (
	flagCacheJSON       bool
	flagCacheLicenseKey string
	flagCacheAll        bool
)

func init() {
	cacheListCmd.Flags().BoolVar(&flagCacheJSON, "json", false, "output as JSON")
	cacheListCmd.Flags().StringVar(&flagCacheLicenseKey, "license", "", "filter by license key")

	cachePurgeCmd.Flags().StringVar(&flagCacheLicenseKey, "license", "", "only purge for specific license key")
	cachePurgeCmd.Flags().BoolVar(&flagCacheAll, "all", false, "confirm purging all cached downloads")

	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cachePurgeCmd)
	cacheCmd.AddCommand(cachePathCmd)

	rootCmd.AddCommand(cacheCmd)
}

func runCacheList(cmd *cobra.Command, args []string) error {
	manager, err := cache.NewManager()
	if err != nil {
		return err
	}

	var entries []*cache.Entry
	if flagCacheLicenseKey != "" {
		entries, err = manager.ListForLicense(flagCacheLicenseKey)
	} else {
		entries, err = manager.List()
	}
	if err != nil {
		return err
	}

	if flagCacheJSON {
		type jsonEntry struct {
			LicenseKey   string `json:"license_key"`
			DownloadID   string `json:"download_id"`
			Version      string `json:"version"`
			Filename     string `json:"filename"`
			Size         int64  `json:"size"`
			SizeHuman    string `json:"size_human"`
			Checksum     string `json:"checksum"`
			DownloadedAt string `json:"downloaded_at"`
			FilePath     string `json:"file_path"`
		}

		jsonEntries := make([]jsonEntry, 0, len(entries))
		for _, e := range entries {
			jsonEntries = append(jsonEntries, jsonEntry{
				LicenseKey:   e.LicenseKey,
				DownloadID:   e.Metadata.DownloadID,
				Version:      e.Metadata.Version,
				Filename:     e.Metadata.Filename,
				Size:         e.Metadata.Size,
				SizeHuman:    ui.FormatBytes(e.Metadata.Size),
				Checksum:     e.Metadata.Checksum,
				DownloadedAt: e.Metadata.DownloadedAt.Format("2006-01-02 15:04:05"),
				FilePath:     e.FilePath,
			})
		}

		data, err := json.MarshalIndent(jsonEntries, "", "  ")
		if err != nil {
			return errors.Wrap(errors.CodeInternal, "failed to marshal cache list", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if len(entries) == 0 {
		ui.PrintInfo("No cached downloads found.")
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LicenseKey != entries[j].LicenseKey {
			return entries[i].LicenseKey < entries[j].LicenseKey
		}
		if entries[i].Metadata.DownloadID != entries[j].Metadata.DownloadID {
			return entries[i].Metadata.DownloadID < entries[j].Metadata.DownloadID
		}
		return entries[i].Metadata.Version < entries[j].Metadata.Version
	})

	var totalSize int64
	for _, e := range entries {
		totalSize += e.Metadata.Size
	}

	if flagVerbose {
		return runCacheListVerbose(entries, totalSize)
	}

	return runCacheListTable(entries, totalSize)
}

func runCacheListTable(entries []*cache.Entry, totalSize int64) error {
	fmt.Printf("%s Cached downloads: %s entries, %s total\n\n",
		ui.StatusIcon("info"),
		ui.Bold.Render(fmt.Sprintf("%d", len(entries))),
		ui.Bold.Render(ui.FormatBytes(totalSize)))

	headers := []string{"LICENSE", "PRODUCT", "VERSION", "SIZE", "DOWNLOADED"}
	rows := make([][]string, 0, len(entries))

	for _, e := range entries {
		rows = append(rows, []string{
			e.LicenseKey,
			e.Metadata.DownloadID,
			ui.Version.Render("v" + e.Metadata.Version),
			ui.FormatBytes(e.Metadata.Size),
			e.Metadata.DownloadedAt.Format("2006-01-02"),
		})
	}

	fmt.Println(ui.NewTable(headers, rows))
	fmt.Printf("\nUse %s for detailed information.\n", ui.Command.Render("-v"))

	return nil
}

func runCacheListVerbose(entries []*cache.Entry, totalSize int64) error {
	fmt.Printf("%s Cached downloads: %s entries, %s total\n\n",
		ui.StatusIcon("info"),
		ui.Bold.Render(fmt.Sprintf("%d", len(entries))),
		ui.Bold.Render(ui.FormatBytes(totalSize)))

	currentLicense := ""
	for _, e := range entries {
		if e.LicenseKey != currentLicense {
			if currentLicense != "" {
				fmt.Println()
			}
			fmt.Printf("%s License %s\n", ui.StatusIcon("success"), ui.Bold.Render(e.LicenseKey))
			currentLicense = e.LicenseKey
		}

		fmt.Printf("\n%s%s %s\n", ui.Indent1, ui.Bold.Render(e.Metadata.DownloadID), ui.Version.Render("v"+e.Metadata.Version))

		shortChecksum := e.Metadata.Checksum
		if len(shortChecksum) > 12 {
			shortChecksum = shortChecksum[:12] + "..."
		}

		pairs := []ui.KVPair{
			ui.KV("File", e.Metadata.Filename),
			ui.KV("Size", ui.FormatBytes(e.Metadata.Size)),
			ui.KV("Downloaded", e.Metadata.DownloadedAt.Format("2006-01-02 15:04:05")),
		}
		if shortChecksum != "" {
			pairs = append(pairs, ui.KV("Checksum", shortChecksum))
		}

		ui.PrintKeyValuePaddedWithIndent(pairs, ui.Indent2)
	}

	return nil
}

func runCachePurge(cmd *cobra.Command, args []string) error {
	manager, err := cache.NewManager()
	if err != nil {
		return err
	}

	if flagCacheLicenseKey != "" {
		entries, err := manager.ListForLicense(flagCacheLicenseKey)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			ui.PrintInfo(fmt.Sprintf("No cached downloads found for license %s.", flagCacheLicenseKey))
			return nil
		}

		var totalSize int64
		for _, e := range entries {
			totalSize += e.Metadata.Size
		}

		if err := manager.PurgeLicense(flagCacheLicenseKey); err != nil {
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Purged %d cached download(s) for license %s (%s freed).",
			len(entries), flagCacheLicenseKey, ui.FormatBytes(totalSize)))
		return nil
	}

	if !flagCacheAll {
		ui.PrintWarning("Use --all to confirm purging all cached downloads, or --license to purge a specific license.")
		return nil
	}

	entries, err := manager.List()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		ui.PrintInfo("No cached downloads to purge.")
		return nil
	}

	var totalSize int64
	for _, e := range entries {
		totalSize += e.Metadata.Size
	}

	if err := manager.PurgeAll(); err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("Purged %d cached download(s) (%s freed).", len(entries), ui.FormatBytes(totalSize)))
	return nil
}

func runCachePath(cmd *cobra.Command, args []string) error {
	path, err := cache.GetCachePath()
	if err != nil {
		return err
	}

	fmt.Println(ui.Path.Render(path))
	return nil
}
