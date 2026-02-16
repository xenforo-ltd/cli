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

type DownloadOptions struct {
	LicenseKey       string
	DownloadID       string // e.g., "xenforo", "xfmg"
	Version          string
	URL              string
	ExpectedChecksum string // SHA-256, optional
	ExpectedSize     int64  // 0 = unknown
	Filename         string // overrides Content-Disposition
	SkipCacheCheck   bool
}

type DownloadResult struct {
	Entry           *Entry
	WasCached       bool
	BytesDownloaded int64
}

// ProgressCallback reports download progress; total is -1 if unknown.
type ProgressCallback func(current, total int64)

func (m *Manager) Download(ctx context.Context, opts DownloadOptions, progress ProgressCallback) (*DownloadResult, error) {
	return m.download(ctx, opts, "", progress)
}

func (m *Manager) DownloadWithAuth(ctx context.Context, opts DownloadOptions, authToken string, progress ProgressCallback) (*DownloadResult, error) {
	return m.download(ctx, opts, authToken, progress)
}

func (m *Manager) download(ctx context.Context, opts DownloadOptions, authToken string, progress ProgressCallback) (*DownloadResult, error) {
	if !opts.SkipCacheCheck {
		if result, err := m.checkCache(opts); result != nil || err != nil {
			return result, err
		}
	}

	resp, err := m.doDownloadRequest(ctx, opts.URL, authToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkResponseStatus(resp, authToken); err != nil {
		return nil, err
	}

	filename := resolveFilename(opts.Filename, resp, opts.URL)

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
	downloaded, err := downloadToFile(filePath, resp.Body, totalSize, opts.ExpectedChecksum, progress)
	if err != nil {
		return nil, err
	}

	entry, err := m.finalizeEntry(opts, filePath, entryPath)
	if err != nil {
		os.Remove(filePath)
		return nil, err
	}

	return &DownloadResult{
		Entry:           entry,
		BytesDownloaded: downloaded,
	}, nil
}

func (m *Manager) checkCache(opts DownloadOptions) (*DownloadResult, error) {
	entry, err := m.GetEntry(opts.LicenseKey, opts.DownloadID, opts.Version)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	valid, err := m.Verify(entry)
	if err != nil || !valid {
		return nil, nil
	}
	if _, err := os.Stat(entry.FilePath); err != nil {
		return nil, nil
	}

	return &DownloadResult{Entry: entry, WasCached: true}, nil
}

func (m *Manager) doDownloadRequest(ctx context.Context, url, authToken string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "failed to create download request", err)
	}

	v := version.Get()
	req.Header.Set("User-Agent", fmt.Sprintf("xf/%s (%s/%s)", v.Version, v.OS, v.Arch))

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
		req.Header.Set("Accept", "*/*")
	}

	client := &http.Client{Timeout: 30 * time.Minute}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "download request failed", err)
	}

	return resp, nil
}

func checkResponseStatus(resp *http.Response, authToken string) error {
	if authToken != "" && resp.StatusCode == http.StatusUnauthorized {
		return errors.New(errors.CodeAuthExpired, "authentication expired - run 'xf auth login'")
	}

	if resp.StatusCode != http.StatusOK {
		if authToken != "" {
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 && len(body) < 500 {
				return errors.Newf(errors.CodeDownloadFailed, "download failed with status %d: %s", resp.StatusCode, string(body))
			}
		}
		return errors.Newf(errors.CodeDownloadFailed, "download failed with status %d", resp.StatusCode)
	}

	return nil
}

func resolveFilename(override string, resp *http.Response, url string) string {
	filename := override
	if filename == "" {
		filename = parseFilenameFromResponse(resp, url)
	}
	if safe := sanitizeFilename(filename); safe != "" {
		return safe
	}
	return "download.zip"
}

func downloadToFile(destPath string, src io.Reader, totalSize int64, expectedChecksum string, progress ProgressCallback) (int64, error) {
	tmpPath := destPath + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return 0, errors.Wrap(errors.CodeFileWriteFailed, "failed to create download file", err)
	}

	var downloaded int64
	reader := &stream.ProgressReader{
		Reader: src,
		Total:  totalSize,
		OnProgress: func(current, total int64) {
			downloaded = current
			if progress != nil {
				progress(current, total)
			}
		},
	}

	_, copyErr := io.Copy(f, reader)
	closeErr := f.Close()

	if copyErr != nil {
		os.Remove(tmpPath)
		return 0, errors.Wrap(errors.CodeDownloadFailed, "download interrupted", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return 0, errors.Wrap(errors.CodeFileWriteFailed, "failed to finalize download file", closeErr)
	}

	if expectedChecksum != "" {
		checksum, err := CalculateChecksum(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return 0, err
		}
		if checksum != expectedChecksum {
			os.Remove(tmpPath)
			return 0, errors.Newf(errors.CodeChecksumMismatch,
				"checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return 0, errors.Wrap(errors.CodeFileWriteFailed, "failed to finalize download", err)
	}

	return downloaded, nil
}

func (m *Manager) finalizeEntry(opts DownloadOptions, filePath, entryPath string) (*Entry, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, errors.Wrap(errors.CodeFileReadFailed, "failed to stat downloaded file", err)
	}

	checksum, err := CalculateChecksum(filePath)
	if err != nil {
		return nil, err
	}

	metadata := &EntryMetadata{
		DownloadID:   opts.DownloadID,
		Version:      opts.Version,
		Filename:     filepath.Base(filePath),
		Checksum:     checksum,
		Size:         info.Size(),
		DownloadedAt: time.Now(),
		SourceURL:    opts.URL,
	}

	if err := m.SaveMetadata(opts.LicenseKey, metadata); err != nil {
		return nil, err
	}

	return &Entry{
		LicenseKey:   opts.LicenseKey,
		Metadata:     *metadata,
		FilePath:     filePath,
		MetadataPath: filepath.Join(entryPath, MetadataFilename),
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
