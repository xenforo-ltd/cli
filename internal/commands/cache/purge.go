package cache

import (
	"context"
	"errors"
	"fmt"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// PurgeOptions selects a license or all cached downloads for removal.
type PurgeOptions struct {
	LicenseKey string
	All        bool
}

// Purge removes cached downloads for a license or, when requested, all licenses.
func Purge(_ context.Context, opts PurgeOptions) error {
	manager, err := cache.NewManager()
	if err != nil {
		return fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	if opts.LicenseKey != "" {
		entries, err := manager.ListForLicense(opts.LicenseKey)
		if err != nil {
			return fmt.Errorf("failed to list cached downloads for license %s: %w", opts.LicenseKey, err)
		}

		if len(entries) == 0 {
			ui.PrintEmpty("No cached downloads for license " + ui.Bold.Render(opts.LicenseKey))
			return nil
		}

		if err := manager.PurgeLicense(opts.LicenseKey); err != nil {
			return fmt.Errorf("failed to purge cached downloads for license %s: %w", opts.LicenseKey, err)
		}

		printPurged(entries)

		return nil
	}

	if !opts.All {
		return clierror.Usage(errors.New("specify --all to purge everything, or --license <key> for one license"))
	}

	entries, err := manager.List()
	if err != nil {
		return fmt.Errorf("failed to list cached downloads: %w", err)
	}

	if len(entries) == 0 {
		ui.PrintEmpty("No cached downloads to purge")
		return nil
	}

	if err := manager.PurgeAll(); err != nil {
		return fmt.Errorf("failed to purge all cached downloads: %w", err)
	}

	printPurged(entries)

	return nil
}

// printPurged reports the number and total size of removed downloads.
func printPurged(entries []*cache.Entry) {
	ui.SuccessBox(fmt.Sprintf("Purged %s, freeing %s",
		ui.Plural(len(entries), "cached download", "cached downloads"), ui.FormatBytes(totalSize(entries))), nil)
}
