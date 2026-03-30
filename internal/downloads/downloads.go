// Package downloads handles product and add-on download selection and retrieval.
package downloads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/customerapi"
)

var (
	// ErrAPINotFound indicates the requested resource was not found.
	ErrAPINotFound = errors.New("resource not found")

	// ErrInvalidInput indicates invalid input was provided.
	ErrInvalidInput = errors.New("invalid input")
)

type versionClient interface {
	GetLicenseVersions(ctx context.Context, licenseKey string, downloadID string) (*customerapi.LicenseVersions, error)
	GetDownloadInfo(ctx context.Context, licenseKey string, downloadID string, versionID int) (*customerapi.DownloadInfo, error)
}

type downloadClient interface {
	versionClient
	GetAccessToken() (string, error)
	GetDownloadURL(licenseKey string, downloadID string, versionID int) string
}

type cacheDownloader interface {
	GetEntry(licenseKey string, downloadID, version string) (*cache.Entry, error)
	Verify(entry *cache.Entry) (bool, error)
	DownloadWithAuth(ctx context.Context, opts cache.DownloadOptions, authToken string, progress cache.ProgressCallback) (*cache.DownloadResult, error)
}

// Selection represents a product/version selection to download.
type Selection struct {
	Product       string
	VersionID     int
	VersionString string
	Reason        string
}

// ResolveSelections resolves versions for core and selected add-ons.
//
// Rules:
// 1) XenForo core must be explicitly selected by version ID.
// 2) If core is latest, add-ons use latest.
// 3) Otherwise add-ons try exact version string, then date fallback, then latest fallback.
// 4) Optional per-product overrides take precedence.
func ResolveSelections(
	ctx context.Context,
	client versionClient,
	licenseKey string,
	products []string,
	coreVersionID int,
	coreVersionString string,
	overrides map[string]int,
	onSkip func(product string),
) ([]Selection, error) {
	selections := make([]Selection, 0, len(products))

	if overrides == nil {
		overrides = map[string]int{}
	}

	coreVersions, err := client.GetLicenseVersions(ctx, licenseKey, "xenforo")
	if err != nil {
		return nil, fmt.Errorf("failed to get versions for xenforo: %w", err)
	}

	if len(coreVersions.Versions) == 0 {
		return nil, fmt.Errorf("no xenforo versions available: %w", ErrAPINotFound)
	}

	sortVersions(coreVersions.Versions)
	latestCore := coreVersions.Versions[0]

	var selectedCore *customerapi.Version

	for i := range coreVersions.Versions {
		v := &coreVersions.Versions[i]
		if v.VersionID == coreVersionID {
			selectedCore = v
			break
		}
	}

	for _, product := range products {
		if product == "xenforo" {
			if coreVersionID == 0 {
				return nil, fmt.Errorf("xenforo core version ID is required: %w", ErrInvalidInput)
			}

			versionStr := coreVersionString
			reason := "selected core version"

			if selectedCore != nil {
				versionStr = selectedCore.VersionStr
				if selectedCore.VersionID == latestCore.VersionID {
					reason = "latest core"
				}
			}

			selections = append(selections, Selection{
				Product:       product,
				VersionID:     coreVersionID,
				VersionString: versionStr,
				Reason:        reason,
			})

			continue
		}

		if overrideID := overrides[product]; overrideID > 0 {
			info, err := client.GetDownloadInfo(ctx, licenseKey, product, overrideID)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve override for %s: %w", product, err)
			}

			selections = append(selections, Selection{
				Product:       product,
				VersionID:     overrideID,
				VersionString: info.VersionString,
				Reason:        "manual override",
			})

			continue
		}

		versions, err := client.GetLicenseVersions(ctx, licenseKey, product)
		if err != nil {
			return nil, fmt.Errorf("failed to get versions for %s: %w", product, err)
		}

		if len(versions.Versions) == 0 {
			if onSkip != nil {
				onSkip(product)
			}

			continue
		}

		sortVersions(versions.Versions)
		selected := resolveAddonSelection(versions.Versions, selectedCore, &latestCore, coreVersionString)
		selections = append(selections, Selection{
			Product:       product,
			VersionID:     selected.VersionID,
			VersionString: selected.VersionStr,
			Reason:        selected.Reason,
		})
	}

	return selections, nil
}

type resolvedVersion struct {
	VersionID  int
	VersionStr string
	Reason     string
}

func resolveAddonSelection(addonVersions []customerapi.Version, selectedCore, latestCore *customerapi.Version, coreVersionString string) resolvedVersion {
	latestAddon := addonVersions[0]
	if selectedCore != nil && latestCore != nil && selectedCore.VersionID == latestCore.VersionID {
		return resolvedVersion{
			VersionID:  latestAddon.VersionID,
			VersionStr: latestAddon.VersionStr,
			Reason:     "latest (core is latest)",
		}
	}

	coreVersionForMatch := coreVersionString
	if selectedCore != nil && strings.TrimSpace(selectedCore.VersionStr) != "" {
		coreVersionForMatch = selectedCore.VersionStr
	}

	normCore := normalizeVersion(coreVersionForMatch)
	if normCore != "" {
		for _, v := range addonVersions {
			if normalizeVersion(v.VersionStr) == normCore {
				return resolvedVersion{
					VersionID:  v.VersionID,
					VersionStr: v.VersionStr,
					Reason:     "exact match",
				}
			}
		}
	}

	if selectedCore != nil {
		best := newestAtOrBefore(addonVersions, selectedCore.ReleaseDate.Time)
		if best != nil {
			return resolvedVersion{
				VersionID:  best.VersionID,
				VersionStr: best.VersionStr,
				Reason:     "date-match fallback",
			}
		}
	}

	return resolvedVersion{
		VersionID:  latestAddon.VersionID,
		VersionStr: latestAddon.VersionStr,
		Reason:     "latest fallback (no <= release date)",
	}
}

func sortVersions(v []customerapi.Version) {
	sort.Slice(v, func(i, j int) bool {
		ti := v[i].ReleaseDate.Time

		tj := v[j].ReleaseDate.Time
		if !ti.Equal(tj) {
			return ti.After(tj)
		}

		return v[i].VersionID > v[j].VersionID
	})
}

func newestAtOrBefore(versions []customerapi.Version, t time.Time) *customerapi.Version {
	if t.IsZero() {
		return nil
	}

	var picked *customerapi.Version

	for i := range versions {
		v := &versions[i]
		if v.ReleaseDate.IsZero() || v.ReleaseDate.After(t) {
			continue
		}

		if picked == nil {
			picked = v
			continue
		}

		if v.ReleaseDate.After(picked.ReleaseDate.Time) ||
			(v.ReleaseDate.Equal(picked.ReleaseDate.Time) && v.VersionID > picked.VersionID) {
			picked = v
		}
	}

	return picked
}

func normalizeVersion(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))

	s = strings.TrimPrefix(s, "v")
	if id, err := strconv.Atoi(s); err == nil && id > 0 {
		return strconv.Itoa(id)
	}

	return s
}

// DownloadSelection downloads a product based on the selection.
func DownloadSelection(ctx context.Context, client *customerapi.Client, cacheManager *cache.Manager, licenseKey string, selection Selection, skipCache bool, progress cache.ProgressCallback) (*cache.Entry, string, error) {
	return downloadSelection(ctx, client, cacheManager, licenseKey, selection, skipCache, progress)
}

func downloadSelection(ctx context.Context, client downloadClient, cacheManager cacheDownloader, licenseKey string, selection Selection, skipCache bool, progress cache.ProgressCallback) (*cache.Entry, string, error) {
	info, err := client.GetDownloadInfo(ctx, licenseKey, selection.Product, selection.VersionID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get download info for %s: %w", selection.Product, err)
	}

	versionStr := selection.VersionString
	if versionStr == "" {
		versionStr = info.VersionString
	}

	if !skipCache {
		entry, err := cacheManager.GetEntry(licenseKey, selection.Product, versionStr)
		if err != nil && !errors.Is(err, cache.ErrCacheMiss) {
			return nil, "", fmt.Errorf("failed to check cache for %s %s: %w", selection.Product, versionStr, err)
		}

		if entry != nil {
			valid, err := cacheManager.Verify(entry)
			if err == nil && valid {
				if _, statErr := os.Stat(entry.FilePath); statErr == nil {
					return entry, versionStr, nil
				}
			}
		}
	}

	accessToken, err := client.GetAccessToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get access token for %s: %w", selection.Product, err)
	}

	downloadURL := client.GetDownloadURL(licenseKey, selection.Product, selection.VersionID)
	downloadOpts := cache.DownloadOptions{
		LicenseKey:     licenseKey,
		DownloadID:     selection.Product,
		Version:        versionStr,
		URL:            downloadURL,
		Filename:       info.Filename,
		SkipCacheCheck: skipCache,
	}

	result, err := cacheManager.DownloadWithAuth(ctx, downloadOpts, accessToken, progress)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download %s: %w", selection.Product, err)
	}

	return result.Entry, versionStr, nil
}
