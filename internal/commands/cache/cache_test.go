package cache_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"

	downloadcache "github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/clierror"
	cachecommand "github.com/xenforo-ltd/cli/internal/commands/cache"
	"github.com/xenforo-ltd/cli/internal/config"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "xf-cache-commands-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	run := func() int {
		cachePath := filepath.Join(dir, "cache")
		if err := os.Setenv("XF_CACHE_PATH", cachePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, fmt.Appendf(nil, `{"cache_path":%q}`, cachePath), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := config.Init(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return m.Run()
	}
	code := run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func captureOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	originalWriter := lipgloss.Writer
	writer := *originalWriter
	writer.Forward = f
	os.Stdout = f
	lipgloss.Writer = &writer
	defer func() {
		os.Stdout = original
		lipgloss.Writer = originalWriter
		f.Close()
	}()
	runErr := run()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(output), runErr
}

func populatedCache(t *testing.T) *downloadcache.Manager {
	t.Helper()
	m, err := downloadcache.NewManager()
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "archive contents")
	}))
	t.Cleanup(server.Close)
	for _, license := range []string{"license-a", "license-b"} {
		_, err := m.Download(t.Context(), downloadcache.DownloadOptions{
			LicenseKey: license, DownloadID: "xenforo", Version: "2.3.8", URL: server.URL, Filename: "release.zip",
		}, "", nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func TestListJSONFiltersLicenseWithoutChangingCache(t *testing.T) {
	m := populatedCache(t)
	for _, license := range []string{"", "license-b", "missing"} {
		out, err := captureOutput(t, func() error {
			return cachecommand.List(t.Context(), cachecommand.ListOptions{JSON: true, LicenseKey: license})
		})
		if err != nil {
			t.Fatal(err)
		}
		var entries []struct {
			LicenseKey string `json:"license_key"`
			DownloadID string `json:"download_id"`
			Version    string `json:"version"`
			FilePath   string `json:"file_path"`
		}
		if err := json.Unmarshal([]byte(out), &entries); err != nil || entries == nil {
			t.Fatalf("list must produce a JSON array: %q, %v", out, err)
		}
		wantCount := map[string]int{"": 2, "license-b": 1, "missing": 0}[license]
		if len(entries) != wantCount {
			t.Fatalf("license %q: got %d entries, want %d", license, len(entries), wantCount)
		}
		for _, entry := range entries {
			if license != "" && entry.LicenseKey != license {
				t.Errorf("filter %q returned license %q", license, entry.LicenseKey)
			}
			if entry.DownloadID != "xenforo" || entry.Version != "2.3.8" {
				t.Errorf("wrong download identity: %+v", entry)
			}
			if data, err := os.ReadFile(entry.FilePath); err != nil || string(data) != "archive contents" {
				t.Errorf("listed file is not usable: %q, %v", data, err)
			}
		}
	}
	if entries, err := m.List(); err != nil || len(entries) != 2 {
		t.Fatalf("listing changed cache: %v, %v", entries, err)
	}
}

func TestPurgeRequiresExplicitScopeAndPreservesOtherLicenses(t *testing.T) {
	m := populatedCache(t)
	err := cachecommand.Purge(t.Context(), cachecommand.PurgeOptions{})
	if !clierror.IsUsage(err) {
		t.Fatalf("unscoped purge must be an invocation error: %v", err)
	}
	if err := cachecommand.Purge(t.Context(), cachecommand.PurgeOptions{LicenseKey: ".."}); err == nil {
		t.Fatal("accepted license path escaping cache")
	}
	if entries, err := m.List(); err != nil || len(entries) != 2 {
		t.Fatalf("rejected purge changed cache: %v, %v", entries, err)
	}
	if err := cachecommand.Purge(t.Context(), cachecommand.PurgeOptions{LicenseKey: "license-a"}); err != nil {
		t.Fatal(err)
	}
	entries, err := m.List()
	if err != nil || len(entries) != 1 || entries[0].LicenseKey != "license-b" {
		t.Fatalf("selective purge removed the wrong entries: %v, %v", entries, err)
	}
	if err := cachecommand.Purge(t.Context(), cachecommand.PurgeOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	if entries, err := m.List(); err != nil || len(entries) != 0 {
		t.Fatalf("all-cache purge left entries: %v, %v", entries, err)
	}
}
