package extract

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizePathTraversal(t *testing.T) {
	_, err := sanitizePath("/tmp/dest", "../evil")
	if err == nil {
		t.Fatal("expected traversal path to be rejected")
	}

	_, err = sanitizePath("/tmp/dest", "/abs/evil")
	if err == nil {
		t.Fatal("expected absolute path to be rejected")
	}

	_, err = sanitizePath("/tmp/dest", "C:\\evil")
	if err == nil {
		t.Fatal("expected Windows drive path to be rejected")
	}
}

func TestSanitizePathWindowsTraversal(t *testing.T) {
	_, err := sanitizePath("C:\\dest", "..\\evil")
	if err == nil {
		t.Fatal("expected Windows-style traversal to be rejected")
	}
}

func TestZipFileRejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(os.ModeSymlink | 0o755)

	file, err := zw.CreateHeader(header)
	if err != nil {
		zw.Close()
		t.Fatalf("create header: %v", err)
	}

	_, _ = file.Write([]byte("target"))

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	err = ZipFile(zipPath, filepath.Join(tmpDir, "out"), DefaultOptions())
	if err == nil {
		t.Fatal("expected symlink entry to be rejected")
	}

	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestExtractFileOverwriteBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "file.zip")

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	w, err := zw.Create("upload/src/XF.php")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}

	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	dest := filepath.Join(tmpDir, "XF.php")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := extractFile(reader.File[0], dest, &Options{OverwriteExisting: false, PreservePermissions: true}); err != nil {
		t.Fatalf("extractFile overwrite=false failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	if string(data) != "old" {
		t.Fatalf("expected old content, got %q", string(data))
	}

	if err := extractFile(reader.File[0], dest, &Options{OverwriteExisting: true, PreservePermissions: true}); err != nil {
		t.Fatalf("extractFile overwrite=true failed: %v", err)
	}

	data, err = os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	if string(data) != "new" {
		t.Fatalf("expected new content, got %q", string(data))
	}
}

func TestGetZipRootDirectory(t *testing.T) {
	t.Run("single root", func(t *testing.T) {
		zipPath := filepath.Join(t.TempDir(), "single.zip")
		if err := writeZip(zipPath, map[string]string{
			"upload/src/XF.php": "x",
			"upload/js/app.js":  "y",
		}); err != nil {
			t.Fatalf("write zip: %v", err)
		}

		root, err := GetZipRootDirectory(zipPath)
		if err != nil {
			t.Fatalf("GetZipRootDirectory failed: %v", err)
		}

		if root != "upload" {
			t.Fatalf("root = %q, want upload", root)
		}
	})

	t.Run("mixed roots", func(t *testing.T) {
		zipPath := filepath.Join(t.TempDir(), "mixed.zip")
		if err := writeZip(zipPath, map[string]string{
			"upload/src/XF.php": "x",
			"docs/readme.txt":   "y",
		}); err != nil {
			t.Fatalf("write zip: %v", err)
		}

		root, err := GetZipRootDirectory(zipPath)
		if err != nil {
			t.Fatalf("GetZipRootDirectory failed: %v", err)
		}

		if root != "" {
			t.Fatalf("root = %q, want empty", root)
		}
	})
}

func TestExtractXenForoZipUploadOnly(t *testing.T) {
	tmpDir := t.TempDir()

	zipPath := filepath.Join(tmpDir, "xf.zip")
	if err := writeZip(zipPath, map[string]string{
		"upload/src/XF.php": "xf",
		"upload/js/app.js":  "js",
		"README.txt":        "skip-me",
	}); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")

	var filenames []string

	if err := XenForoZip(zipPath, outDir, func(_, _ int, name string) {
		filenames = append(filenames, name)
	}); err != nil {
		t.Fatalf("ExtractXenForoZip failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "src", "XF.php")); err != nil {
		t.Fatalf("expected src/XF.php: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "js", "app.js")); err != nil {
		t.Fatalf("expected js/app.js: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "README.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected root README not extracted, err=%v", err)
	}

	if len(filenames) == 0 {
		t.Fatal("expected progress callback filenames")
	}
}

func writeZip(zipPath string, files map[string]string) error {
	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}

		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}

	return os.WriteFile(zipPath, buf.Bytes(), 0o644)
}
