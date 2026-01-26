package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"xf/internal/errors"
	"xf/internal/stream"
	"xf/internal/version"
)

// DownloadOptions configures a download operation.
type DownloadOptions struct {
	// LicenseKey is the license the download is associated with.
	LicenseKey string

	// DownloadID identifies the product (e.g., "xenforo", "xfmg").
	DownloadID string

	// Version is the version string.
	Version string

	// URL is the download URL.
	URL string

	// ExpectedChecksum is the expected SHA-256 checksum (optional).
	ExpectedChecksum string

	// ExpectedSize is the expected file size in bytes (optional, 0 = unknown).
	ExpectedSize int64

	// Filename overrides the filename from Content-Disposition (optional).
	Filename string

	// SkipCacheCheck bypasses cache lookup (force re-download).
	SkipCacheCheck bool
}

// DownloadResult contains the result of a download operation.
type DownloadResult struct {
	// Entry is the cache entry for the downloaded file.
	Entry *Entry

	// WasCached indicates if the file was served from cache.
	WasCached bool

	// BytesDownloaded is the number of bytes downloaded (0 if cached).
	BytesDownloaded int64
}

// ProgressCallback is called during download to report progress.
// current is bytes downloaded, total is total bytes (-1 if unknown).
type ProgressCallback func(current, total int64)

func (m *Manager) Download(ctx context.Context, opts DownloadOptions, progress ProgressCallback) (*DownloadResult, error) {
	if !opts.SkipCacheCheck {
		entry, err := m.GetEntry(opts.LicenseKey, opts.DownloadID, opts.Version)
		if err != nil {
			return nil, err
		}

		if entry != nil {
			valid, err := m.Verify(entry)
			if err == nil && valid {
				if _, err := os.Stat(entry.FilePath); err == nil {
					return &DownloadResult{
						Entry:     entry,
						WasCached: true,
					}, nil
				}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "failed to create download request", err)
	}

	v := version.Get()
	req.Header.Set("User-Agent", fmt.Sprintf("xf/%s (%s/%s)", v.Version, v.OS, v.Arch))

	client := &http.Client{
		Timeout: 30 * time.Minute, // Long timeout for large downloads
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "download request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf(errors.CodeDownloadFailed, "download failed with status %d", resp.StatusCode)
	}

	filename := opts.Filename
	if filename == "" {
		filename = parseFilenameFromResponse(resp, opts.URL)
	}
	if safe := sanitizeFilename(filename); safe != "" {
		filename = safe
	} else {
		filename = "download.zip"
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 && opts.ExpectedSize > 0 {
		totalSize = opts.ExpectedSize
	}

	entryPath, err := m.EntryPath(opts.LicenseKey, opts.DownloadID, opts.Version)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(entryPath, 0755); err != nil {
		return nil, errors.Wrap(errors.CodeDirCreateFailed, "failed to create cache directory", err)
	}

	filePath := filepath.Join(entryPath, filename)
	tmpPath := filePath + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, errors.Wrap(errors.CodeFileWriteFailed, "failed to create download file", err)
	}

	var downloaded int64
	reader := &stream.ProgressReader{
		Reader: resp.Body,
		Total:  totalSize,
		OnProgress: func(current, total int64) {
			downloaded = current
			if progress != nil {
				progress(current, total)
			}
		},
	}

	_, err = io.Copy(f, reader)
	closeErr := f.Close()

	if err != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeDownloadFailed, "download interrupted", err)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeFileWriteFailed, "failed to finalize download file", closeErr)
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeFileReadFailed, "failed to stat downloaded file", err)
	}

	checksum, err := CalculateChecksum(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}

	if opts.ExpectedChecksum != "" && checksum != opts.ExpectedChecksum {
		os.Remove(tmpPath)
		return nil, errors.Newf(errors.CodeChecksumMismatch,
			"checksum mismatch: expected %s, got %s", opts.ExpectedChecksum, checksum)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeFileWriteFailed, "failed to finalize download", err)
	}

	metadata := &EntryMetadata{
		DownloadID:   opts.DownloadID,
		Version:      opts.Version,
		Filename:     filename,
		Checksum:     checksum,
		Size:         info.Size(),
		DownloadedAt: time.Now(),
		SourceURL:    opts.URL,
	}

	if err := m.SaveMetadata(opts.LicenseKey, metadata); err != nil {
		return nil, err
	}

	entry := &Entry{
		LicenseKey:   opts.LicenseKey,
		Metadata:     *metadata,
		FilePath:     filePath,
		MetadataPath: filepath.Join(entryPath, MetadataFilename),
	}

	return &DownloadResult{
		Entry:           entry,
		WasCached:       false,
		BytesDownloaded: downloaded,
	}, nil
}

func parseFilenameFromResponse(resp *http.Response, url string) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "filename=") {
				filename := strings.TrimPrefix(part, "filename=")
				filename = strings.Trim(filename, "\"")
				if filename != "" {
					if safe := sanitizeFilename(filename); safe != "" {
						return safe
					}
					break
				}
			}
		}
	}

	parts := strings.Split(url, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			name := strings.Split(parts[i], "?")[0]
			if name != "" {
				if safe := sanitizeFilename(name); safe != "" {
					return safe
				}
				break
			}
		}
	}

	return "download.zip"
}

func sanitizeFilename(name string) string {
	clean := filepath.Base(name)
	if clean == "." || clean == string(filepath.Separator) || clean == "" {
		return ""
	}
	if strings.ContainsAny(clean, `/\\`) {
		return ""
	}
	return clean
}

// This is used when the download URL requires authentication.
func (m *Manager) DownloadWithAuth(ctx context.Context, opts DownloadOptions, authToken string, progress ProgressCallback) (*DownloadResult, error) {
	if !opts.SkipCacheCheck {
		entry, err := m.GetEntry(opts.LicenseKey, opts.DownloadID, opts.Version)
		if err != nil {
			return nil, err
		}

		if entry != nil {
			valid, err := m.Verify(entry)
			if err == nil && valid {
				if _, err := os.Stat(entry.FilePath); err == nil {
					return &DownloadResult{
						Entry:     entry,
						WasCached: true,
					}, nil
				}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "failed to create download request", err)
	}

	v := version.Get()
	req.Header.Set("User-Agent", fmt.Sprintf("xf/%s (%s/%s)", v.Version, v.OS, v.Arch))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("Accept", "*/*")

	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "download request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New(errors.CodeAuthExpired, "authentication expired - run 'xf auth login'")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 && len(body) < 500 {
			return nil, errors.Newf(errors.CodeDownloadFailed, "download failed with status %d: %s", resp.StatusCode, string(body))
		}
		return nil, errors.Newf(errors.CodeDownloadFailed, "download failed with status %d", resp.StatusCode)
	}

	filename := opts.Filename
	if filename == "" {
		filename = parseFilenameFromResponse(resp, opts.URL)
	}
	if safe := sanitizeFilename(filename); safe != "" {
		filename = safe
	} else {
		filename = "download.zip"
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 && opts.ExpectedSize > 0 {
		totalSize = opts.ExpectedSize
	}

	entryPath, err := m.EntryPath(opts.LicenseKey, opts.DownloadID, opts.Version)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(entryPath, 0755); err != nil {
		return nil, errors.Wrap(errors.CodeDirCreateFailed, "failed to create cache directory", err)
	}

	filePath := filepath.Join(entryPath, filename)
	tmpPath := filePath + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, errors.Wrap(errors.CodeFileWriteFailed, "failed to create download file", err)
	}

	var downloaded int64
	reader := &stream.ProgressReader{
		Reader: resp.Body,
		Total:  totalSize,
		OnProgress: func(current, total int64) {
			downloaded = current
			if progress != nil {
				progress(current, total)
			}
		},
	}

	_, err = io.Copy(f, reader)
	closeErr := f.Close()

	if err != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeDownloadFailed, "download interrupted", err)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeFileWriteFailed, "failed to finalize download file", closeErr)
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeFileReadFailed, "failed to stat downloaded file", err)
	}

	checksum, err := CalculateChecksum(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}

	if opts.ExpectedChecksum != "" && checksum != opts.ExpectedChecksum {
		os.Remove(tmpPath)
		return nil, errors.Newf(errors.CodeChecksumMismatch,
			"checksum mismatch: expected %s, got %s", opts.ExpectedChecksum, checksum)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return nil, errors.Wrap(errors.CodeFileWriteFailed, "failed to finalize download", err)
	}

	metadata := &EntryMetadata{
		DownloadID:   opts.DownloadID,
		Version:      opts.Version,
		Filename:     filename,
		Checksum:     checksum,
		Size:         info.Size(),
		DownloadedAt: time.Now(),
		SourceURL:    opts.URL,
	}

	if err := m.SaveMetadata(opts.LicenseKey, metadata); err != nil {
		return nil, err
	}

	entry := &Entry{
		LicenseKey:   opts.LicenseKey,
		Metadata:     *metadata,
		FilePath:     filePath,
		MetadataPath: filepath.Join(entryPath, MetadataFilename),
	}

	return &DownloadResult{
		Entry:           entry,
		WasCached:       false,
		BytesDownloaded: downloaded,
	}, nil
}
