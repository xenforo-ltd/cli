package extract_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xenforo-ltd/cli/internal/extract"
)

type zipEntry struct {
	name, content string
	mode          os.FileMode
}

func makeZip(t *testing.T, entries ...zipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			h.SetMode(entry.mode)
		}
		out, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestXenForoZip(t *testing.T) {
	archive := makeZip(t, zipEntry{name: "README.txt", content: "ignored"}, zipEntry{name: "upload/"}, zipEntry{name: "upload/src/"}, zipEntry{name: "upload/src/file.txt", content: "hello"})
	dest := filepath.Join(t.TempDir(), "destination")
	last := 0
	fileNotified := false
	err := extract.XenForoZip(archive, dest, func(current, total int, filename string) {
		if current <= last || current < 1 || current > total {
			t.Errorf("unexpected progress: %d/%d %s", current, total, filename)
		}
		last = current
		if filename == "src/file.txt" {
			fileNotified = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fileNotified {
		t.Fatal("no progress notification for extracted file")
	}
	if data, err := os.ReadFile(filepath.Join(dest, "src", "file.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("extracted content: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.txt")); !os.IsNotExist(err) {
		t.Fatalf("root file was extracted: %v", err)
	}
}

func TestXenForoZipRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{"upload/../escape", "upload//absolute", `upload/C:/escape`, `upload/dir\escape`} {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "destination")
			if err := extract.XenForoZip(makeZip(t, zipEntry{name: name, content: "bad"}), dest, nil); err == nil {
				t.Fatalf("accepted unsafe path %q", name)
			}
		})
	}
	t.Run("archive symlink", func(t *testing.T) {
		archive := makeZip(t, zipEntry{name: "upload/link", content: "target", mode: os.ModeSymlink | 0o777})
		if err := extract.XenForoZip(archive, t.TempDir(), nil); err == nil {
			t.Fatal("accepted symlink")
		}
	})
	t.Run("destination symlink", func(t *testing.T) {
		dest, outside := t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("cannot create symlink: %v", err)
			}
			t.Fatal(err)
		}
		archive := makeZip(t, zipEntry{name: "upload/link/escape", content: "bad"})
		if err := extract.XenForoZip(archive, dest, nil); err == nil {
			t.Fatal("followed destination symlink outside root")
		}
		if _, err := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
			t.Fatalf("outside file exists: %v", err)
		}
	})
}

func TestXenForoZipInvalidArchive(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid.zip")
	if err := os.WriteFile(invalid, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{invalid, filepath.Join(dir, "missing.zip")} {
		if err := extract.XenForoZip(path, filepath.Join(dir, "dest"), nil); err == nil {
			t.Errorf("accepted archive %q", path)
		}
	}
}

func TestXenForoZipRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	header := &zip.FileHeader{Name: "upload/large", Method: zip.Deflate}
	out, err := w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write(bytes.Repeat([]byte("x"), 32*1024*1024+1)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extract.XenForoZip(path, filepath.Join(dir, "dest"), nil); err == nil {
		t.Fatal("accepted oversized file")
	}
}
