package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"xf/internal/errors"
	"xf/internal/stream"
	"xf/internal/version"
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
)

var (
	executablePath  = os.Executable
	evaluateSymlink = filepath.EvalSymlinks
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

func NewUpdater() *Updater {
	return &Updater{
		GitHubOwner: DefaultGitHubOwner,
		GitHubRepo:  DefaultGitHubRepo,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

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
			return nil, errors.Newf(errors.CodeUpdateFailed,
				"no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	}

	return info, nil
}

func (u *Updater) Update(ctx context.Context, info *UpdateInfo, progressFn func(downloaded, total int64)) error {
	if !info.HasUpdate {
		return errors.New(errors.CodeUpdateFailed, "no update available")
	}
	if info.AssetURL == "" || info.AssetName == "" {
		return errors.New(errors.CodeUpdateFailed, "update asset information is incomplete")
	}

	execPath, err := executablePath()
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to get executable path", err)
	}
	execPath, err = evaluateSymlink(execPath)
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to resolve executable path", err)
	}

	// Create a temporary working directory in the binary directory to keep renames atomic.
	tmpDir, err := os.MkdirTemp(filepath.Dir(execPath), "xf-update-*")
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to create temporary directory", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, info.AssetName)
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to create update archive", err)
	}

	if err := u.downloadFile(ctx, info.AssetURL, archiveFile, progressFn); err != nil {
		archiveFile.Close()
		return err
	}
	if err := archiveFile.Close(); err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to finalize update archive", err)
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

	if runtime.GOOS != "windows" {
		if err := os.Chmod(newBinaryPath, 0755); err != nil {
			return errors.Wrap(errors.CodeUpdateFailed, "failed to set permissions on new binary", err)
		}
	}

	// Perform atomic replacement.
	// On Unix, we can rename over the existing file.
	// On Windows, we need to move the old file first.
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		os.Remove(oldPath) // Remove any existing .old file.
		if err := os.Rename(execPath, oldPath); err != nil {
			return errors.Wrap(errors.CodeUpdateFailed, "failed to backup old binary", err)
		}
		if err := os.Rename(newBinaryPath, execPath); err != nil {
			// Try to restore the old binary.
			os.Rename(oldPath, execPath)
			return errors.Wrap(errors.CodeUpdateFailed, "failed to replace binary", err)
		}
		os.Remove(oldPath) // Clean up the old binary.
	} else {
		if err := os.Rename(newBinaryPath, execPath); err != nil {
			return errors.Wrap(errors.CodeUpdateFailed, "failed to replace binary", err)
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
		return "", errors.Newf(errors.CodeUpdateFailed, "unsupported update archive format: %s", archivePath)
	}
}

func extractBinaryFromTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", errors.Wrap(errors.CodeUpdateFailed, "failed to open update archive", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return "", errors.Wrap(errors.CodeUpdateFailed, "failed to read update archive", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	extracted := make(map[string]string)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to read update archive", err)
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		name := path.Base(header.Name)
		if !isBinaryCandidate(name) {
			continue
		}

		outPath := filepath.Join(destDir, name)
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to extract update binary", err)
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			outFile.Close()
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to extract update binary", err)
		}
		if err := outFile.Close(); err != nil {
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to finalize update binary", err)
		}

		mode := header.FileInfo().Mode().Perm()
		if mode != 0 && runtime.GOOS != "windows" {
			if err := os.Chmod(outPath, mode); err != nil {
				return "", errors.Wrap(errors.CodeUpdateFailed, "failed to apply binary permissions", err)
			}
		}

		extracted[name] = outPath
	}

	return pickExtractedBinary(extracted)
}

func extractBinaryFromZip(archivePath, destDir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", errors.Wrap(errors.CodeUpdateFailed, "failed to open update archive", err)
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

		inFile, err := file.Open()
		if err != nil {
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to read update binary from archive", err)
		}

		outPath := filepath.Join(destDir, name)
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			inFile.Close()
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to extract update binary", err)
		}

		if _, err := io.Copy(outFile, inFile); err != nil {
			outFile.Close()
			inFile.Close()
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to extract update binary", err)
		}
		if err := outFile.Close(); err != nil {
			inFile.Close()
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to finalize update binary", err)
		}
		if err := inFile.Close(); err != nil {
			return "", errors.Wrap(errors.CodeUpdateFailed, "failed to close update archive entry", err)
		}

		mode := file.Mode().Perm()
		if mode != 0 && runtime.GOOS != "windows" {
			if err := os.Chmod(outPath, mode); err != nil {
				return "", errors.Wrap(errors.CodeUpdateFailed, "failed to apply binary permissions", err)
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
	if runtime.GOOS == "windows" {
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

	return "", errors.New(errors.CodeUpdateFailed, "update archive did not contain xf binary")
}

func parseChecksumForAsset(data []byte, assetName string) (string, bool) {
	singleValueChecksum := ""
	singleValueCount := 0

	for _, line := range strings.Split(string(data), "\n") {
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
		return nil, errors.Wrap(errors.CodeUpdateFailed, "failed to create request", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "xf/"+version.Get().Version)

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(errors.CodeNetworkFailed, "failed to check for updates", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New(errors.CodeUpdateFailed, "no releases found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf(errors.CodeUpdateFailed, "GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, errors.Wrap(errors.CodeUpdateFailed, "failed to parse release info", err)
	}

	return &release, nil
}

// downloadFile downloads a file from the given URL.
func (u *Updater) downloadFile(ctx context.Context, url string, dest *os.File, progressFn func(downloaded, total int64)) error {
	ctx, cancel := context.WithTimeout(ctx, DownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to create download request", err)
	}
	req.Header.Set("User-Agent", "xf/"+version.Get().Version)

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(errors.CodeNetworkFailed, "failed to download update", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Newf(errors.CodeUpdateFailed, "download failed with status %d", resp.StatusCode)
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
		return errors.Wrap(errors.CodeUpdateFailed, "failed to save update", err)
	}

	return nil
}

// verifyChecksum verifies the downloaded file against the published checksum.
func (u *Updater) verifyChecksum(ctx context.Context, filePath string, info *UpdateInfo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.ChecksumURL, nil)
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to create checksum request", err)
	}
	req.Header.Set("User-Agent", "xf/"+version.Get().Version)

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(errors.CodeNetworkFailed, "failed to download checksum", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Newf(errors.CodeChecksumMismatch, "failed to download checksum (status %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to read checksum", err)
	}

	expectedChecksum, ok := parseChecksumForAsset(body, info.AssetName)
	if !ok {
		return errors.Newf(errors.CodeChecksumMismatch, "checksum entry missing for asset %s", info.AssetName)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to open downloaded file for verification", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrap(errors.CodeUpdateFailed, "failed to calculate checksum", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return errors.Newf(errors.CodeChecksumMismatch,
			"checksum verification failed: expected %s, got %s", expectedChecksum, actualChecksum)
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
	if goos == "windows" {
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
	maxLen := len(latestParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}

	for i := 0; i < maxLen; i++ {
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
		fmt.Sscanf(p, "%d", &result[i])
	}
	return result
}
