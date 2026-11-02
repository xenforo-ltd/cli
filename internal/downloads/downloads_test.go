package downloads_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
)

func TestSelectionAndDownload(t *testing.T) {
	if os.Getenv("XF_DOWNLOADS_TEST_CHILD") != "1" {
		// Configuration is cached for the process lifetime.
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSelectionAndDownload$")
		if dir := os.Getenv("GOCOVERDIR"); dir != "" {
			cmd.Args = append(cmd.Args, "-test.gocoverdir="+dir)
		}
		cmd.Env = append(os.Environ(), "XF_DOWNLOADS_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated test failed: %v\n%s", err, output)
		}
		return
	}

	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	f, err := zw.Create("upload/addon.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("selected add-on")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	versions := map[string]string{
		"xenforo": `[{"version_id":20,"version_string":"2.3.0","release_date":"2025-06-01T00:00:00Z"},{"version_id":10,"version_string":"2.2.0","release_date":"2024-06-01T00:00:00Z"}]`,
		"exact":   `[{"version_id":32,"version_string":"3.0.0","release_date":"2025-07-01T00:00:00Z"},{"version_id":31,"version_string":" V2.2.0 ","release_date":"2023-01-01T00:00:00Z"}]`,
		"date":    `[{"version_id":42,"version_string":"3.0.0","release_date":"2025-01-01T00:00:00Z"},{"version_id":40,"version_string":"1.0.0","release_date":"2023-01-01T00:00:00Z"},{"version_id":41,"version_string":"1.5.0","release_date":"2024-06-01T00:00:00Z"}]`,
		"future":  `[{"version_id":52,"version_string":"3.0.0","release_date":"2025-01-01T00:00:00Z"},{"version_id":51,"version_string":"2.5.0","release_date":"2024-07-01T00:00:00Z"}]`,
		"empty":   `[]`,
	}
	var noCoreVersions atomic.Bool
	downloadedVersion := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-token" || r.URL.Query().Get("license_key") != "license-1" {
			t.Errorf("unexpected request authentication or license: %s", r.URL)
		}
		product := r.URL.Query().Get("download_id")
		switch r.URL.Path {
		case "/api/customer-oauth2/license-versions":
			if product == "xenforo" && noCoreVersions.Load() {
				fmt.Fprint(w, `{"versions":[]}`)
				return
			}
			if product == "failure" {
				http.Error(w, "versions unavailable", http.StatusServiceUnavailable)
				return
			}
			data, ok := versions[product]
			if !ok {
				t.Errorf("unexpected versions request for %q", product)
				http.Error(w, "unexpected product", http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, `{"versions":%s}`, data)
		case "/api/customer-oauth2/license-download-info":
			if r.URL.Query().Get("version_id") == "999" {
				http.Error(w, "override unavailable", http.StatusNotFound)
				return
			}
			fmt.Fprint(w, `{"download":{"version_string":"override-release","filename":"addon.zip"}}`)
		case "/api/customer-oauth2/license-download":
			downloadedVersion <- r.URL.Query().Get("version_id")
			_, _ = w.Write(archive.Bytes())
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("XF_TOKEN", "local-token")
	configFile := filepath.Join(t.TempDir(), "config.json")
	cachePath := t.TempDir()
	t.Setenv("XF_OAUTH_BASE_URL", server.URL)
	t.Setenv("XF_CACHE_PATH", cachePath)
	configData := fmt.Sprintf(`{"oauth":{"base_url":%q},"cache_path":%q}`, server.URL, cachePath)
	if err := os.WriteFile(configFile, []byte(configData), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}
	client, err := customerapi.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("latest core selects latest add-on", func(t *testing.T) {
		got, err := downloads.ResolveSelections(ctx, client, "license-1", []string{"xenforo", "exact"}, 20, "ignored", nil, nil)
		if err != nil || len(got) != 2 {
			t.Fatalf("selections: %v, %v", got, err)
		}
		if got[0].Product != "xenforo" || got[0].VersionString != "2.3.0" || got[1].Product != "exact" || got[1].VersionString != "3.0.0" {
			t.Fatalf("unexpected selections: %+v", got)
		}
	})

	t.Run("older core selects exact then date then latest", func(t *testing.T) {
		var skipped []string
		got, err := downloads.ResolveSelections(ctx, client, "license-1", []string{"xenforo", "exact", "date", "future", "empty"}, 10, "wrong", nil, func(product string) { skipped = append(skipped, product) })
		if err != nil || len(got) != 4 {
			t.Fatalf("selections: %v, %v", got, err)
		}
		want := []string{"2.2.0", " V2.2.0 ", "1.5.0", "3.0.0"}
		for i, version := range want {
			if got[i].VersionString != version {
				t.Errorf("selection %d: got %+v, want version %q", i, got[i], version)
			}
		}
		if !reflect.DeepEqual(skipped, []string{"empty"}) {
			t.Errorf("skipped: %v", skipped)
		}
		manager, err := cache.NewManager()
		if err != nil {
			t.Fatal(err)
		}
		entry, version, err := downloads.DownloadSelection(ctx, client, manager, "license-1", got[2], false, nil)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		var requested string
		select {
		case requested = <-downloadedVersion:
		default:
			t.Fatal("no download request received")
		}
		if version != "1.5.0" || requested != "41" {
			t.Fatalf("download: version=%q requested ID=%q", version, requested)
		}
		dest := t.TempDir()
		if err := downloads.ExtractCachedFiles(map[string]*cache.Entry{"date": entry}, dest, nil, "Extracted"); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dest, "addon.txt"))
		if err != nil || string(data) != "selected add-on" {
			t.Fatalf("extracted: %q, %v", data, err)
		}
	})

	t.Run("override takes precedence", func(t *testing.T) {
		got, err := downloads.ResolveSelections(ctx, client, "license-1", []string{"empty"}, 20, "2.3.0", map[string]int{"empty": 77}, nil)
		if err != nil || len(got) != 1 || got[0].Product != "empty" || got[0].VersionString != "override-release" {
			t.Fatalf("override: %+v, %v", got, err)
		}
	})

	t.Run("no core versions is an error", func(t *testing.T) {
		noCoreVersions.Store(true)
		defer noCoreVersions.Store(false)
		_, err := downloads.ResolveSelections(ctx, client, "license-1", []string{"empty"}, 10, "2.2.0", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "no xenforo versions available") {
			t.Fatalf("expected unavailable core error, got %v", err)
		}
	})

	t.Run("missing or unavailable versions", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			products  []string
			core      int
			overrides map[string]int
			message   string
		}{
			{"missing core ID", []string{"xenforo"}, 0, nil, "core version ID is required"},
			{"versions request fails", []string{"failure"}, 10, nil, "failed to get versions for failure"},
			{"override request fails", []string{"empty"}, 10, map[string]int{"empty": 999}, "failed to resolve override for empty"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := downloads.ResolveSelections(ctx, client, "license-1", tc.products, tc.core, "2.2.0", tc.overrides, nil)
				if err == nil || !strings.Contains(err.Error(), tc.message) {
					t.Fatalf("expected %q error, got %v", tc.message, err)
				}
			})
		}
	})
}
