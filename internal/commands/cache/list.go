package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

type ListOptions struct {
	JSON       bool
	LicenseKey string
}

func List(_ context.Context, opts ListOptions) error {
	manager, err := cache.NewManager()
	if err != nil {
		return fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	var entries []*cache.Entry
	if opts.LicenseKey != "" {
		entries, err = manager.ListForLicense(opts.LicenseKey)
	} else {
		entries, err = manager.List()
	}

	if err != nil {
		return fmt.Errorf("failed to list cached downloads: %w", err)
	}

	if opts.JSON {
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
			return fmt.Errorf("failed to marshal cache list: %w", err)
		}

		ui.Println(string(data))

		return nil
	}

	if len(entries) == 0 {
		ui.PrintEmpty("No cached downloads found")
		return nil
	}

	slices.SortFunc(entries, func(a, b *cache.Entry) int {
		if c := strings.Compare(a.LicenseKey, b.LicenseKey); c != 0 {
			return c
		}

		if c := strings.Compare(a.Metadata.DownloadID, b.Metadata.DownloadID); c != 0 {
			return c
		}

		return strings.Compare(a.Metadata.Version, b.Metadata.Version)
	})

	var totalSize int64
	for _, e := range entries {
		totalSize += e.Metadata.Size
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.Verbose {
		return runCacheListVerbose(entries, totalSize)
	}

	return runCacheListTable(entries, totalSize)
}
