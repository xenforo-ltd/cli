// Package selfupdate provides CLI self-update functionality using GitHub releases.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/stream"
	"github.com/xenforo-ltd/cli/internal/version"
)

var (
	// ErrUpdateFailed indicates the update operation failed.
	ErrUpdateFailed = errors.New("update failed")

	// ErrChecksumMismatch indicates a checksum verification failure.
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

const (
	// DefaultGitHubOwner is the default GitHub organization.
	DefaultGitHubOwner = "xenforo-ltd"

	// DefaultGitHubRepo is the default GitHub repository name.
	DefaultGitHubRepo = "cli"

	// GitHubAPIBase is the base URL for GitHub API.
	GitHubAPIBase = "https://api.github.com"

	// DownloadTimeout is the timeout for downloading updates.
	DownloadTimeout = 5 * time.Minute

	maxBinarySize = 32 * 1024 * 1024 // 32 MB

	windowsOS = "windows"
)

// Release represents a GitHub release.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	Assets      []Asset   `json:"assets"`
	HTMLURL     string    `json:"html_url"`
}

// Asset represents a release asset (downloadable file).
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// UpdateInfo contains information about an available update.
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	ReleaseNotes   string
	AssetURL       string
	AssetName      string
	ChecksumURL    string
	HasUpdate      bool
}

// Updater handles checking for and applying updates.
type Updater struct {
	GitHubOwner string
	GitHubRepo  string
	HTTPClient  *http.Client
}

// NewUpdater creates a new updater with default GitHub repository settings.
func NewUpdater() *Updater {
	return &Updater{
		GitHubOwner: DefaultGitHubOwner,
		GitHubRepo:  DefaultGitHubRepo,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CheckForUpdate checks if an update is available on GitHub.
func (u *Updater) CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	release, err := u.getLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	currentVersion := version.Get().Version
	latestVersion := strings.TrimPrefix(release.TagName, "v")

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		ReleaseURL:     release.HTMLURL,
		ReleaseNotes:   release.Body,
		HasUpdate:      false,
	}

	if isNewerVersion(latestVersion, currentVersion) {
		info.HasUpdate = true

		assetName := getArchiveAssetName(release.TagName)
		for _, asset := range release.Assets {
			if asset.Name == assetName {
				info.AssetURL = asset.BrowserDownloadURL
				info.AssetName = asset.Name
			}

			if asset.Name == "checksums.txt" || asset.Name == assetName+".sha256" {
				info.ChecksumURL = asset.BrowserDownloadURL
			}
		}

		if info.AssetURL == "" {
			return nil, fmt.Errorf("no release asset found for %s/%s: %w", runtime.GOOS, runtime.GOARCH, ErrUpdateFailed)
		}
	}

	return info, nil
}

// Update downloads and applies the new version.
func (u *Updater) Update(ctx context.Context, info *UpdateInfo, progressFn func(downloaded, total int64)) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	return u.updateAtPath(ctx, execPath, info, progressFn)
}

func (u *Updater) updateAtPath(ctx context.Context, execPath string, info *UpdateInfo, progressFn func(downloaded, total int64)) error {
	if !info.HasUpdate {
		return fmt.Errorf("no update available: %w", ErrUpdateFailed)
	}

	if info.AssetURL == "" || info.AssetName == "" {
		return fmt.Errorf("update asset information is incomplete: %w", ErrUpdateFailed)
	}

	// Create a temporary working directory in the binary directory to keep renames atomic.
	tmpDir, err := os.MkdirTemp(filepath.Dir(execPath), "xf-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	archivePath := filepath.Join(tmpDir, info.AssetName)

	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("failed to create update archive: %w", err)
	}

	if err := u.downloadFile(ctx, info.AssetURL, archiveFile, progressFn); err != nil {
		archiveFile.Close()
		return err
	}

	if err := archiveFile.Close(); err != nil {
		return fmt.Errorf("failed to finalize update archive: %w", err)
	}

	if info.ChecksumURL != "" {
		if err := u.verifyChecksum(ctx, archivePath, info); err != nil {
			return err
		}
	}

	newBinaryPath, err := extractBinaryFromArchive(archivePath, tmpDir)
	if err != nil {
		return err
	}

	if runtime.GOOS != windowsOS {
		if err := os.Chmod(newBinaryPath, 0o700); err != nil {
			return fmt.Errorf("failed to set permissions on new binary: %w", err)
		}
	}

	// Perform atomic replacement.
	// On Unix, we can rename over the existing file.
	// On Windows, we need to move the old file first.
	if runtime.GOOS == windowsOS {
		oldPath := execPath + ".old"

		// Remove any existing .old file; ignore "not exist" since that's expected.
		if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove stale backup: %w", err)
		}

		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to backup old binary: %w", err)
		}

		if err := os.Rename(newBinaryPath, execPath); err != nil {
			// Try to restore the old binary.
			restoreErr := os.Rename(oldPath, execPath)
			return errors.Join(fmt.Errorf("failed to replace binary: %w", err), restoreErr)
		}

		// Clean up the old binary; ignore "not exist" errors.
		if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove old binary: %w", err)
		}
	} else {
		if err := os.Rename(newBinaryPath, execPath); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
	}

	return nil
}

func extractBinaryFromArchive(archivePath, destDir string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractBinaryFromTarGz(archivePath, destDir)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractBinaryFromZip(archivePath, destDir)
	default:
		return "", fmt.Errorf("unsupported update archive format: %s: %w", archivePath, ErrUpdateFailed)
	}
}

func extractBinaryFromTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open update archive: %w", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("failed to read update archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	extracted := make(map[string]string)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("failed to read update archive: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		name := path.Base(header.Name)
		if !isBinaryCandidate(name) {
			continue
		}

		if header.Size > maxBinarySize {
			return "", fmt.Errorf("update binary %s exceeds maximum allowed size of %d bytes: %w", name, maxBinarySize, ErrUpdateFailed)
		}

		outPath := filepath.Join(destDir, name)
		limitedReader := io.LimitReader(tarReader, maxBinarySize)

		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return "", fmt.Errorf("failed to extract update binary: %w", err)
		}

		written, err := io.Copy(outFile, limitedReader)
		if err != nil {
			outFile.Close()
			return "", fmt.Errorf("failed to extract update binary: %w", err)
		}

		if written > maxBinarySize {
			outFile.Close()
			return "", fmt.Errorf("update binary %s exceeds maximum allowed size of %d bytes: %w", name, maxBinarySize, ErrUpdateFailed)
		}

		if err := outFile.Close(); err != nil {
			return "", fmt.Errorf("failed to finalize update binary: %w", err)
		}

		mode := header.FileInfo().Mode().Perm()
		if mode != 0 && runtime.GOOS != windowsOS {
			if err := os.Chmod(outPath, mode); err != nil {
				return "", fmt.Errorf("failed to apply binary permissions: %w", err)
			}
		}

		extracted[name] = outPath
	}

	return pickExtractedBinary(extracted)
}

func extractBinaryFromZip(archivePath, destDir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open update archive: %w", err)
	}
	defer reader.Close()

	extracted := make(map[string]string)

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		name := path.Base(file.Name)
		if !isBinaryCandidate(name) {
			continue
		}

		if file.UncompressedSize64 > maxBinarySize {
			return "", fmt.Errorf("update binary %s exceeds maximum allowed size of %d bytes: %w", name, maxBinarySize, ErrUpdateFailed)
		}

		inFile, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("failed to read update binary from archive: %w", err)
		}

		outPath := filepath.Join(destDir, name)
		limitedReader := io.LimitReader(inFile, maxBinarySize)

		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			inFile.Close()
			return "", fmt.Errorf("failed to extract update binary: %w", err)
		}

		written, err := io.Copy(outFile, limitedReader)
		if err != nil {
			outFile.Close()
			inFile.Close()

			return "", fmt.Errorf("failed to extract update binary: %w", err)
		}

		if written > maxBinarySize {
			outFile.Close()
			inFile.Close()

			return "", fmt.Errorf("update binary %s exceeds maximum allowed size of %d bytes: %w", name, maxBinarySize, ErrUpdateFailed)
		}

		if err := outFile.Close(); err != nil {
			inFile.Close()
			return "", fmt.Errorf("failed to finalize update binary: %w", err)
		}

		if err := inFile.Close(); err != nil {
			return "", fmt.Errorf("failed to close update archive entry: %w", err)
		}

		mode := file.Mode().Perm()
		if mode != 0 && runtime.GOOS != windowsOS {
			if err := os.Chmod(outPath, mode); err != nil {
				return "", fmt.Errorf("failed to apply binary permissions: %w", err)
			}
		}

		extracted[name] = outPath
	}

	return pickExtractedBinary(extracted)
}

func isBinaryCandidate(name string) bool {
	return name == "xf" || name == "xf.exe"
}

func runtimeBinaryName() string {
	if runtime.GOOS == windowsOS {
		return "xf.exe"
	}

	return "xf"
}

func pickExtractedBinary(extracted map[string]string) (string, error) {
	if binaryPath, ok := extracted[runtimeBinaryName()]; ok {
		return binaryPath, nil
	}

	if binaryPath, ok := extracted["xf"]; ok {
		return binaryPath, nil
	}

	if binaryPath, ok := extracted["xf.exe"]; ok {
		return binaryPath, nil
	}

	return "", fmt.Errorf("update archive did not contain xf binary: %w", ErrUpdateFailed)
}

func parseChecksumForAsset(data []byte, assetName string) (string, bool) {
	singleValueChecksum := ""
	singleValueCount := 0

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 1 {
			singleValueChecksum = parts[0]
			singleValueCount++

			continue
		}

		if len(parts) < 2 {
			continue
		}

		filename := strings.TrimPrefix(parts[len(parts)-1], "*")
		if filename == assetName || filepath.Base(filename) == assetName {
			return parts[0], true
		}
	}

	if singleValueCount == 1 {
		return singleValueChecksum, true
	}

	return "", false
}

// getLatestRelease fetches the latest non-prerelease release from GitHub.
func (u *Updater) getLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", GitHubAPIBase, u.GitHubOwner, u.GitHubRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "github.com/xenforo-ltd/cli/"+version.Get().Version)

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found: %w", ErrUpdateFailed)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d: %w", resp.StatusCode, ErrUpdateFailed)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	return &release, nil
}

// downloadFile downloads a file from the given URL.
func (u *Updater) downloadFile(ctx context.Context, url string, dest *os.File, progressFn func(downloaded, total int64)) error {
	ctx, cancel := context.WithTimeout(ctx, DownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	req.Header.Set("User-Agent", "github.com/xenforo-ltd/cli/"+version.Get().Version)

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d: %w", resp.StatusCode, ErrUpdateFailed)
	}

	var reader io.Reader = resp.Body
	if progressFn != nil {
		reader = &stream.ProgressReader{
			Reader:     resp.Body,
			Total:      resp.ContentLength,
			OnProgress: progressFn,
		}
	}

	_, err = io.Copy(dest, reader)
	if err != nil {
		return fmt.Errorf("failed to save update: %w", err)
	}

	return nil
}

// verifyChecksum verifies the downloaded file against the published checksum.
func (u *Updater) verifyChecksum(ctx context.Context, filePath string, info *UpdateInfo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.ChecksumURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create checksum request: %w", err)
	}

	req.Header.Set("User-Agent", "github.com/xenforo-ltd/cli/"+version.Get().Version)

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download checksum (status %d): %w", resp.StatusCode, ErrChecksumMismatch)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read checksum: %w", err)
	}

	expectedChecksum, ok := parseChecksumForAsset(body, info.AssetName)
	if !ok {
		return fmt.Errorf("checksum entry missing for asset %s: %w", info.AssetName, ErrChecksumMismatch)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded file for verification: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return fmt.Errorf("checksum verification failed: expected %s, got %s: %w", expectedChecksum, actualChecksum, ErrChecksumMismatch)
	}

	return nil
}

func getArchiveAssetName(versionTag string) string {
	return getArchiveAssetNameForPlatform(versionTag, runtime.GOOS, runtime.GOARCH)
}

func getArchiveAssetNameForPlatform(versionTag, goos, goarch string) string {
	tag := strings.TrimSpace(versionTag)
	if tag == "" {
		tag = version.Get().Version
	}

	if tag != "" && !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}

	ext := ".tar.gz"
	if goos == windowsOS {
		ext = ".zip"
	}

	return fmt.Sprintf("xf-%s-%s-%s%s", tag, goos, goarch, ext)
}

// isNewerVersion compares two semantic versions.
// Returns true if latest is newer than current.
func isNewerVersion(latest, current string) bool {
	// Handle "dev" version - always consider updates available.
	if current == "dev" || current == "unknown" {
		return true
	}

	latestParts := parseVersion(latest)
	currentParts := parseVersion(current)

	// Pad the shorter slice with zeros for proper comparison.
	// This ensures 1.0 == 1.0.0 and 1.0.1 > 1.0
	maxLen := max(len(currentParts), len(latestParts))

	for i := range maxLen {
		var l, c int
		if i < len(latestParts) {
			l = latestParts[i]
		}

		if i < len(currentParts) {
			c = currentParts[i]
		}

		if l > c {
			return true
		}

		if l < c {
			return false
		}
	}

	return false
}

// parseVersion parses a semantic version string into numeric parts.
func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")

	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")

	result := make([]int, len(parts))
	for i, p := range parts {
		if _, err := fmt.Sscanf(p, "%d", &result[i]); err != nil {
			result[i] = 0
		}
	}

	return result
}
