package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/stream"
	"github.com/xenforo-ltd/cli/internal/version"
)

// DownloadOptions specifies download parameters.
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

// DownloadResult contains information about a completed download.
type DownloadResult struct {
	Entry           *Entry
	WasCached       bool
	BytesDownloaded int64
}

// ProgressCallback reports download progress; total is -1 if unknown.
type ProgressCallback func(current, total int64)

// Download downloads and caches a file without authentication.
func (m *Manager) Download(ctx context.Context, opts DownloadOptions, progress ProgressCallback) (*DownloadResult, error) {
	return m.download(ctx, opts, "", progress)
}

// DownloadWithAuth downloads and caches a file with an authentication token.
func (m *Manager) DownloadWithAuth(ctx context.Context, opts DownloadOptions, authToken string, progress ProgressCallback) (*DownloadResult, error) {
	return m.download(ctx, opts, authToken, progress)
}

func (m *Manager) download(ctx context.Context, opts DownloadOptions, authToken string, progress ProgressCallback) (*DownloadResult, error) {
	if !opts.SkipCacheCheck {
		result, err := m.checkCache(opts)
		if err == nil {
			return result, nil
		}

		if !errors.Is(err, ErrCacheMiss) {
			return nil, err
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

	if err := os.MkdirAll(entryPath, 0o755); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create cache directory", err)
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

	valid, err := m.Verify(entry)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, ErrCacheMiss
	}

	if _, err := os.Stat(entry.FilePath); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		return nil, ErrCacheMiss
	}

	return &DownloadResult{Entry: entry, WasCached: true}, nil
}

func (m *Manager) doDownloadRequest(ctx context.Context, url, authToken string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeDownloadFailed, "failed to create download request", err)
	}

	v := version.Get()
	req.Header.Set("User-Agent", fmt.Sprintf("github.com/xenforo-ltd/cli/%s (%s/%s)", v.Version, v.OS, v.Arch))

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
		req.Header.Set("Accept", "*/*")
	}

	client := &http.Client{Timeout: 30 * time.Minute}

	resp, err := client.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeDownloadFailed, "download request failed", err)
	}

	return resp, nil
}

func checkResponseStatus(resp *http.Response, authToken string) error {
	if authToken != "" && resp.StatusCode == http.StatusUnauthorized {
		return clierrors.New(clierrors.CodeAuthExpired, "authentication expired - run 'xf auth login'")
	}

	if resp.StatusCode != http.StatusOK {
		if authToken != "" {
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 && len(body) < 500 {
				return clierrors.Newf(clierrors.CodeDownloadFailed, "download failed with status %d: %s", resp.StatusCode, string(body))
			}
		}

		return clierrors.Newf(clierrors.CodeDownloadFailed, "download failed with status %d", resp.StatusCode)
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
		return 0, clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to create download file", err)
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
		return 0, clierrors.Wrap(clierrors.CodeDownloadFailed, "download interrupted", copyErr)
	}

	if closeErr != nil {
		os.Remove(tmpPath)
		return 0, clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to finalize download file", closeErr)
	}

	if expectedChecksum != "" {
		checksum, err := CalculateChecksum(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return 0, err
		}

		if checksum != expectedChecksum {
			os.Remove(tmpPath)

			return 0, clierrors.Newf(clierrors.CodeChecksumMismatch,
				"checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return 0, clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to finalize download", err)
	}

	return downloaded, nil
}

func (m *Manager) finalizeEntry(opts DownloadOptions, filePath, entryPath string) (*Entry, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to stat downloaded file", err)
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
		for part := range strings.SplitSeq(cd, ";") {
			part = strings.TrimSpace(part)
			if after, ok := strings.CutPrefix(part, "filename="); ok {
				filename := after

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
