package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/version"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		{"newer major", "2.0.0", "1.0.0", true},
		{"newer minor", "1.1.0", "1.0.0", true},
		{"newer patch", "1.0.1", "1.0.0", true},
		{"same version", "1.0.0", "1.0.0", false},
		{"older major", "1.0.0", "2.0.0", false},
		{"older minor", "1.0.0", "1.1.0", false},
		{"older patch", "1.0.0", "1.0.1", false},

		{"v prefix latest", "v1.1.0", "1.0.0", true},
		{"v prefix current", "1.1.0", "v1.0.0", true},
		{"v prefix both", "v1.1.0", "v1.0.0", true},

		{"dev version", "1.0.0", "dev", true},
		{"unknown version", "1.0.0", "unknown", true},

		{"longer newer", "1.0.1", "1.0", true},
		{"longer same", "1.0.0", "1.0", false},

		{"with prerelease", "1.1.0-beta.1", "1.0.0", true},
		{"same with prerelease", "1.0.0-beta.1", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    []int
	}{
		{"1.0.0", []int{1, 0, 0}},
		{"v1.2.3", []int{1, 2, 3}},
		{"2.10.15", []int{2, 10, 15}},
		{"1.0", []int{1, 0}},
		{"1", []int{1}},
		{"1.0.0-beta.1", []int{1, 0, 0}},
		{"v2.3.4+build.123", []int{2, 3, 4}},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersion(tt.version)
			if len(got) != len(tt.want) {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.version, got, tt.want)
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseVersion(%q) = %v, want %v", tt.version, got, tt.want)
					return
				}
			}
		})
	}
}

func TestGetArchiveAssetNameForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		goos     string
		goarch   string
		expected string
	}{
		{
			name:     "linux tar.gz",
			tag:      "v1.2.3",
			goos:     "linux",
			goarch:   "amd64",
			expected: "xf-v1.2.3-linux-amd64.tar.gz",
		},
		{
			name:     "windows zip",
			tag:      "v1.2.3",
			goos:     "windows",
			goarch:   "amd64",
			expected: "xf-v1.2.3-windows-amd64.zip",
		},
		{
			name:     "adds missing v prefix",
			tag:      "1.2.3",
			goos:     "darwin",
			goarch:   "arm64",
			expected: "xf-v1.2.3-darwin-arm64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getArchiveAssetNameForPlatform(tt.tag, tt.goos, tt.goarch)
			if got != tt.expected {
				t.Fatalf("asset name = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParseChecksumForAsset(t *testing.T) {
	data := []byte("abc123  xf-v1.0.0-linux-amd64.tar.gz\ndef456  xf-v1.0.0-darwin-arm64.tar.gz\n")

	checksum, ok := parseChecksumForAsset(data, "xf-v1.0.0-darwin-arm64.tar.gz")
	if !ok {
		t.Fatal("expected checksum match")
	}

	if checksum != "def456" {
		t.Fatalf("checksum = %q, want %q", checksum, "def456")
	}

	single, ok := parseChecksumForAsset([]byte("singlevalue\n"), "any")
	if !ok || single != "singlevalue" {
		t.Fatalf("single checksum parse failed: got=%q ok=%v", single, ok)
	}

	_, ok = parseChecksumForAsset(data, "missing.tar.gz")
	if ok {
		t.Fatal("expected no checksum match")
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "xf-v1.0.0-linux-amd64.tar.gz")

	binaryContent := []byte("new-binary")
	if err := os.WriteFile(archivePath, makeTarGzArchive(t, "xf", binaryContent), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	extractedPath, err := extractBinaryFromArchive(archivePath, tmp)
	if err != nil {
		t.Fatalf("extract archive: %v", err)
	}

	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}

	if !bytes.Equal(data, binaryContent) {
		t.Fatalf("binary content mismatch")
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "xf-v1.0.0-windows-amd64.zip")

	binaryContent := []byte("new-binary-windows")
	if err := os.WriteFile(archivePath, makeZipArchive(t, "xf.exe", binaryContent), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	extractedPath, err := extractBinaryFromArchive(archivePath, tmp)
	if err != nil {
		t.Fatalf("extract archive: %v", err)
	}

	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}

	if !bytes.Equal(data, binaryContent) {
		t.Fatalf("binary content mismatch")
	}
}

func TestVerifyChecksumFailsWhenAssetEntryMissing(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "file.tar.gz")
	if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("abc123  other-file.tar.gz\n"))
	}))
	defer server.Close()

	updater := &Updater{HTTPClient: server.Client()}

	err := updater.verifyChecksum(context.Background(), archive, &UpdateInfo{
		AssetName:   "wanted-file.tar.gz",
		ChecksumURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}

	if !clierrors.Is(err, clierrors.CodeChecksumMismatch) {
		t.Fatalf("expected checksum mismatch code, got: %v", err)
	}
}

func TestVerifyChecksumFailsWhenChecksumUnavailable(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "file.tar.gz")
	if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	updater := &Updater{HTTPClient: server.Client()}

	err := updater.verifyChecksum(context.Background(), archive, &UpdateInfo{
		AssetName:   "wanted-file.tar.gz",
		ChecksumURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected checksum failure")
	}

	if !clierrors.Is(err, clierrors.CodeChecksumMismatch) {
		t.Fatalf("expected checksum mismatch code, got: %v", err)
	}
}

func TestVerifyChecksumFailsWhenChecksumMalformed(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "file.tar.gz")
	if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("###\n\n"))
	}))
	defer server.Close()

	updater := &Updater{HTTPClient: server.Client()}

	err := updater.verifyChecksum(context.Background(), archive, &UpdateInfo{
		AssetName:   "wanted-file.tar.gz",
		ChecksumURL: server.URL,
	})
	if err == nil {
		t.Fatal("expected checksum failure")
	}

	if !clierrors.Is(err, clierrors.CodeChecksumMismatch) {
		t.Fatalf("expected checksum mismatch code, got: %v", err)
	}
}

func TestCheckForUpdateSelectsReleaseArchive(t *testing.T) {
	oldVersion := version.Version
	version.Version = "1.0.0"

	defer func() { version.Version = oldVersion }()

	tag := "v2.0.0"
	assetName := getArchiveAssetNameForPlatform(tag, runtime.GOOS, runtime.GOARCH)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases/latest" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.com/release","body":"notes","assets":[{"name":%q,"browser_download_url":"https://example.com/asset"},{"name":"checksums.txt","browser_download_url":"https://example.com/checksums.txt"}]}`,
			tag, assetName)
	}))
	defer server.Close()

	srvURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	client := &http.Client{Transport: &rewriteHostTransport{
		base:   http.DefaultTransport,
		host:   srvURL.Host,
		scheme: srvURL.Scheme,
	}}

	updater := &Updater{
		GitHubOwner: "o",
		GitHubRepo:  "r",
		HTTPClient:  client,
	}

	info, err := updater.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("check for update: %v", err)
	}

	if !info.HasUpdate {
		t.Fatal("expected update to be available")
	}

	if info.AssetName != assetName {
		t.Fatalf("asset name = %q, want %q", info.AssetName, assetName)
	}

	if info.ChecksumURL == "" {
		t.Fatal("expected checksum URL to be detected")
	}
}

func TestUpdateWithArchiveReplacesExecutable(t *testing.T) {
	tmp := t.TempDir()
	execPath := filepath.Join(tmp, runtimeBinaryName())

	oldContent := []byte("old-binary")
	if err := os.WriteFile(execPath, oldContent, 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}

	archiveName := getArchiveAssetNameForPlatform("v9.9.9", runtime.GOOS, runtime.GOARCH)
	newContent := []byte("new-binary-content")
	archiveData := makeArchiveForName(t, archiveName, runtimeBinaryName(), newContent)
	archiveChecksum := checksumHex(archiveData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			_, _ = w.Write(archiveData)
		case "/checksums":
			_, _ = fmt.Fprintf(w, "%s  %s\n", archiveChecksum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldExecPathFn := executablePath
	oldEvalFn := evaluateSymlink
	executablePath = func() (string, error) { return execPath, nil }
	evaluateSymlink = func(path string) (string, error) { return path, nil }

	defer func() {
		executablePath = oldExecPathFn
		evaluateSymlink = oldEvalFn
	}()

	updater := &Updater{HTTPClient: server.Client()}

	err := updater.Update(context.Background(), &UpdateInfo{
		HasUpdate:   true,
		AssetURL:    server.URL + "/asset",
		AssetName:   archiveName,
		ChecksumURL: server.URL + "/checksums",
	}, nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	finalContent, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}

	if !bytes.Equal(finalContent, newContent) {
		t.Fatalf("binary content was not replaced")
	}
}

func checksumHex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func makeArchiveForName(t *testing.T, archiveName, binaryName string, binaryContent []byte) []byte {
	t.Helper()

	if strings.HasSuffix(archiveName, ".tar.gz") {
		return makeTarGzArchive(t, binaryName, binaryContent)
	}

	if strings.HasSuffix(archiveName, ".zip") {
		return makeZipArchive(t, binaryName, binaryContent)
	}

	t.Fatalf("unsupported archive extension for %s", archiveName)

	return nil
}

func makeTarGzArchive(t *testing.T, binaryName string, binaryContent []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	header := &tar.Header{
		Name: binaryName,
		Mode: 0o755,
		Size: int64(len(binaryContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}

	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatalf("write tar body: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func makeZipArchive(t *testing.T, binaryName string, binaryContent []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	writer, err := zw.Create(binaryName)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}

	if _, err := writer.Write(binaryContent); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return buf.Bytes()
}

type rewriteHostTransport struct {
	base   http.RoundTripper
	host   string
	scheme string
}

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Host = t.host
	cloned.URL.Scheme = t.scheme

	return t.base.RoundTrip(cloned)
}
