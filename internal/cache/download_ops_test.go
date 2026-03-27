package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

func TestDownloadAndUseCache(t *testing.T) {
	m := &Manager{basePath: t.TempDir()}
	payload := []byte("download-payload")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="xf.zip"`)

		if _, err := w.Write(payload); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	opts := DownloadOptions{
		LicenseKey: "LIC1",
		DownloadID: "xenforo",
		Version:    "2.3.8",
		URL:        server.URL + "/file",
	}

	var calls int

	res, err := m.Download(t.Context(), opts, func(_, _ int64) { calls++ })
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if res.WasCached {
		t.Fatal("expected first download not cached")
	}

	if calls == 0 {
		t.Fatal("expected progress callback calls")
	}

	if _, err := os.Stat(res.Entry.FilePath); err != nil {
		t.Fatalf("expected downloaded file: %v", err)
	}

	res2, err := m.Download(t.Context(), opts, nil)
	if err != nil {
		t.Fatalf("Download second call failed: %v", err)
	}

	if !res2.WasCached {
		t.Fatal("expected second download to use cache")
	}
}

func TestDownloadWithChecksumMismatch(t *testing.T) {
	m := &Manager{basePath: t.TempDir()}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("payload")); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	_, err := m.Download(t.Context(), DownloadOptions{
		LicenseKey:       "LIC1",
		DownloadID:       "xenforo",
		Version:          "2.3.8",
		URL:              server.URL,
		ExpectedChecksum: "deadbeef",
	}, nil)
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}

	if !clierrors.Is(err, clierrors.CodeChecksumMismatch) {
		t.Fatalf("expected checksum mismatch code, got: %v", err)
	}
}

func TestDownloadWithAuthUnauthorized(t *testing.T) {
	m := &Manager{basePath: t.TempDir()}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := m.DownloadWithAuth(t.Context(), DownloadOptions{
		LicenseKey: "LIC1",
		DownloadID: "xenforo",
		Version:    "2.3.8",
		URL:        server.URL,
	}, "token", nil)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}

	if !clierrors.Is(err, clierrors.CodeAuthExpired) {
		t.Fatalf("expected auth expired code, got: %v", err)
	}
}

func TestDownloadWithAuthSuccessAndBodyErrorMessage(t *testing.T) {
	m := &Manager{basePath: t.TempDir()}
	payload := []byte("authed-download")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer bad" {
			w.WriteHeader(http.StatusForbidden)

			if _, err := w.Write([]byte("forbidden")); err != nil {
				t.Errorf("write response: %v", err)
			}

			return
		}

		w.Header().Set("Content-Disposition", `attachment; filename="secure.zip"`)

		if _, err := w.Write(payload); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	_, err := m.DownloadWithAuth(t.Context(), DownloadOptions{
		LicenseKey: "LIC1",
		DownloadID: "xenforo",
		Version:    "2.3.8",
		URL:        server.URL,
	}, "bad", nil)
	if err == nil {
		t.Fatal("expected forbidden error")
	}

	if !clierrors.Is(err, clierrors.CodeDownloadFailed) {
		t.Fatalf("expected download failed code, got: %v", err)
	}

	sum := sha256.Sum256(payload)
	expected := hex.EncodeToString(sum[:])

	res, err := m.DownloadWithAuth(t.Context(), DownloadOptions{
		LicenseKey:       "LIC1",
		DownloadID:       "xenforo",
		Version:          "2.3.9",
		URL:              server.URL,
		ExpectedChecksum: expected,
		ExpectedSize:     int64(len(payload)),
	}, "good", nil)
	if err != nil {
		t.Fatalf("DownloadWithAuth success failed: %v", err)
	}

	if res.WasCached {
		t.Fatal("expected non-cached result")
	}

	if got := res.Entry.Metadata.Filename; got != "secure.zip" {
		t.Fatalf("filename = %q, want secure.zip", got)
	}
}

func TestDownloadWithAuthUsesCache(t *testing.T) {
	m := &Manager{basePath: t.TempDir()}
	meta := &EntryMetadata{
		DownloadID:   "xenforo",
		Version:      "2.3.8",
		Filename:     "cached.zip",
		DownloadedAt: time.Now(),
	}

	dir, err := m.EntryPath("LIC1", meta.DownloadID, meta.Version)
	if err != nil {
		t.Fatalf("EntryPath failed: %v", err)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	filePath := filepath.Join(dir, meta.Filename)
	if err := os.WriteFile(filePath, []byte("cached"), 0o600); err != nil {
		t.Fatalf("write cached file failed: %v", err)
	}

	sum, err := CalculateChecksum(filePath)
	if err != nil {
		t.Fatalf("checksum failed: %v", err)
	}

	meta.Checksum = sum

	meta.Size = 6
	if err := m.SaveMetadata("LIC1", meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	res, err := m.DownloadWithAuth(t.Context(), DownloadOptions{
		LicenseKey: "LIC1",
		DownloadID: "xenforo",
		Version:    "2.3.8",
		URL:        "https://example.com/unused",
	}, "token", nil)
	if err != nil {
		t.Fatalf("DownloadWithAuth failed: %v", err)
	}

	if !res.WasCached {
		t.Fatal("expected cached result")
	}
}
