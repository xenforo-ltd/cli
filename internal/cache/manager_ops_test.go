package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{basePath: t.TempDir()}
}

func TestManagerSaveGetVerifyDelete(t *testing.T) {
	m := newTestManager(t)
	license := "ABC123"
	meta := &EntryMetadata{
		DownloadID:   "xenforo",
		Version:      "2.3.8",
		Filename:     "xf.zip",
		DownloadedAt: time.Now(),
	}

	entryPath, err := m.EntryPath(license, meta.DownloadID, meta.Version)
	if err != nil {
		t.Fatalf("EntryPath failed: %v", err)
	}
	if err := os.MkdirAll(entryPath, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	filePath := filepath.Join(entryPath, meta.Filename)
	if err := os.WriteFile(filePath, []byte("archive"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	checksum, err := CalculateChecksum(filePath)
	if err != nil {
		t.Fatalf("CalculateChecksum failed: %v", err)
	}
	meta.Checksum = checksum
	meta.Size = int64(len("archive"))

	if err := m.SaveMetadata(license, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	entry, err := m.GetEntry(license, meta.DownloadID, meta.Version)
	if err != nil {
		t.Fatalf("GetEntry failed: %v", err)
	}
	if entry == nil || entry.FilePath != filePath {
		t.Fatalf("unexpected entry: %#v", entry)
	}

	ok, err := m.Verify(entry)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !ok {
		t.Fatal("expected checksum verification to pass")
	}

	if err := m.Delete(license, meta.DownloadID, meta.Version); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	entry, err = m.GetEntry(license, meta.DownloadID, meta.Version)
	if err != nil {
		t.Fatalf("GetEntry after delete failed: %v", err)
	}
	if entry != nil {
		t.Fatal("expected deleted entry to be nil")
	}
}

func TestManagerListAndTotalSize(t *testing.T) {
	m := newTestManager(t)
	entries := []EntryMetadata{
		{DownloadID: "xenforo", Version: "1", Filename: "a.zip", Size: 10, DownloadedAt: time.Now()},
		{DownloadID: "xfmg", Version: "1", Filename: "b.zip", Size: 20, DownloadedAt: time.Now()},
	}

	for _, meta := range entries {
		license := "LIC1"
		if meta.DownloadID == "xfmg" {
			license = "LIC2"
		}
		dir, err := m.EntryPath(license, meta.DownloadID, meta.Version)
		if err != nil {
			t.Fatalf("EntryPath failed: %v", err)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, meta.Filename), []byte("x"), 0644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
		mcopy := meta
		if err := m.SaveMetadata(license, &mcopy); err != nil {
			t.Fatalf("SaveMetadata failed: %v", err)
		}
	}

	all, err := m.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}

	lic1, err := m.ListForLicense("LIC1")
	if err != nil {
		t.Fatalf("ListForLicense failed: %v", err)
	}
	if len(lic1) != 1 {
		t.Fatalf("len(lic1) = %d, want 1", len(lic1))
	}

	total, err := m.TotalSize()
	if err != nil {
		t.Fatalf("TotalSize failed: %v", err)
	}
	if total != 30 {
		t.Fatalf("total size = %d, want 30", total)
	}
}

func TestManagerPurge(t *testing.T) {
	m := newTestManager(t)
	dir, err := m.EntryPath("LIC1", "xenforo", "1")
	if err != nil {
		t.Fatalf("EntryPath failed: %v", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	if err := m.PurgeLicense("LIC1"); err != nil {
		t.Fatalf("PurgeLicense failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.basePath, "LIC1")); !os.IsNotExist(err) {
		t.Fatalf("expected license path removed, err=%v", err)
	}

	if err := m.PurgeAll(); err != nil {
		t.Fatalf("PurgeAll failed: %v", err)
	}
	if _, err := os.Stat(m.basePath); !os.IsNotExist(err) {
		t.Fatalf("expected base path removed, err=%v", err)
	}
}
