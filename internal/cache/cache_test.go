package cache_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/config"
)

var cacheDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "xf-cache-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cacheDir = filepath.Join(dir, "cache")
	if err := os.Setenv("XF_CACHE_PATH", cacheDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	configPath := filepath.Join(dir, "config.json")
	data := fmt.Appendf(nil, `{"cache_path":%q}`, cacheDir)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	if err := config.Init(configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func TestDownloadLifecycle(t *testing.T) {
	m, err := cache.NewManager()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PurgeAll(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.PurgeAll(); err != nil {
			t.Error(err)
		}
	})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected authorization: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Disposition", `attachment; filename="../release.zip"`)
		fmt.Fprint(w, "release contents")
	}))
	defer server.Close()
	opts := cache.DownloadOptions{LicenseKey: "license-a", DownloadID: "xenforo", Version: "1", URL: server.URL}
	var progress, total int64
	download := func() *cache.DownloadResult {
		t.Helper()
		result, err := m.Download(t.Context(), opts, "test-token", func(current, size int64) {
			progress, total = current, size
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := download()
	if first.WasCached || requests != 1 || progress != int64(len("release contents")) || total != progress {
		t.Fatalf("first download: cached=%v requests=%d progress=%d/%d", first.WasCached, requests, progress, total)
	}
	entry := first.Entry
	sum := sha256.Sum256([]byte("release contents"))
	if entry.LicenseKey != opts.LicenseKey || entry.Metadata.DownloadID != opts.DownloadID || entry.Metadata.Version != opts.Version ||
		entry.Metadata.Filename != "release.zip" || entry.Metadata.Size != int64(len("release contents")) ||
		entry.Metadata.Checksum != fmt.Sprintf("%x", sum) || entry.Metadata.SourceURL != server.URL || entry.Metadata.DownloadedAt.IsZero() {
		t.Fatalf("unexpected entry: %+v, metadata: %+v", entry, entry.Metadata)
	}
	if data, err := os.ReadFile(entry.FilePath); err != nil || string(data) != "release contents" {
		t.Fatalf("downloaded file: %q, %v", data, err)
	}
	if valid, err := m.Verify(entry); err != nil || !valid {
		t.Fatalf("verify: %v, %v", valid, err)
	}
	if result := download(); !result.WasCached || requests != 1 {
		t.Fatalf("cache reuse: cached=%v requests=%d", result.WasCached, requests)
	}
	if err := os.WriteFile(entry.FilePath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if valid, err := m.Verify(entry); err != nil || valid {
		t.Fatalf("corrupt verify: %v, %v", valid, err)
	}
	if valid, err := cache.FindValid(m, opts.LicenseKey, opts.DownloadID, opts.Version); err != nil || valid != nil {
		t.Fatalf("find corrupt entry: %v, %v", valid, err)
	}
	if result := download(); result.WasCached || requests != 2 {
		t.Fatalf("corrupt re-download: cached=%v requests=%d", result.WasCached, requests)
	}
	opts.SkipCacheCheck = true
	if result := download(); result.WasCached || requests != 3 {
		t.Fatalf("forced download: cached=%v requests=%d", result.WasCached, requests)
	}
	opts.LicenseKey = "license-b"
	download()
	if entries, err := m.List(); err != nil || len(entries) != 2 {
		t.Fatalf("list: %d entries, %v", len(entries), err)
	}
	if err := m.PurgeLicense("license-a"); err != nil {
		t.Fatal(err)
	}
	if entries, err := m.ListForLicense("license-a"); err != nil || len(entries) != 0 {
		t.Fatalf("purged license: %d entries, %v", len(entries), err)
	}
	if entries, err := m.ListForLicense("license-b"); err != nil || len(entries) != 1 {
		t.Fatalf("retained license: %d entries, %v", len(entries), err)
	}
	for _, key := range []string{"..", "../../outside"} {
		if err := m.PurgeLicense(key); err == nil {
			t.Errorf("PurgeLicense(%q) succeeded", key)
		}
		if _, err := m.GetEntry(key, "xenforo", "1"); err == nil {
			t.Errorf("GetEntry(%q) succeeded", key)
		}
	}

	t.Run("failures leave no files", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/failure" {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "incomplete")
		}))
		defer server.Close()
		for _, path := range []string{"failure", "incomplete", "cancelled"} {
			opts := cache.DownloadOptions{LicenseKey: "failure-" + path, DownloadID: "xf", Version: "1", URL: server.URL + "/" + path, Filename: "file.zip"}
			ctx := t.Context()
			if path == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := m.Download(ctx, opts, "", nil); err == nil {
				t.Errorf("%s: expected error", path)
			}
			files, err := filepath.Glob(filepath.Join(cacheDir, opts.LicenseKey, "xf", "1", "*"))
			if err != nil || len(files) != 0 {
				t.Errorf("%s: leftover files %v, %v", path, files, err)
			}
			if _, err := m.GetEntry(opts.LicenseKey, "xf", "1"); err == nil || !strings.Contains(err.Error(), "cache miss") {
				t.Errorf("%s: unexpected entry error: %v", path, err)
			}
		}
	})
}
