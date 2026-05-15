package build_test

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	// addOnID is the vendor-prefixed ID used by test fixtures.
	addOnID = "Vendor/Demo"
	// addOnPath is the fixture add-on's installation-relative directory.
	addOnPath = "src/addons/" + addOnID
)

// fixture identifies temporary installation and add-on directories.
type fixture struct {
	root  string
	addon string
}

// newFixture creates an add-on beneath a temporary installation root with the
// given build.json and default metadata.
func newFixture(t *testing.T, config string) fixture {
	t.Helper()
	f := fixture{root: t.TempDir()}
	f.addon = filepath.Join(f.root, filepath.FromSlash(addOnPath))
	writeFile(t, f.root, "src/XF.php", "<?php")
	writeFile(t, f.addon, "addon.json", `{"title":"Demo","version_string":"1.0","version_id":100,"enabled":true}`)
	writeFile(t, f.addon, "build.json", config)
	if err := os.Mkdir(filepath.Join(f.addon, "_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.addon, "_output"), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

// writeFile writes a fixture file, creating parent directories as needed.
func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile reads a fixture file, failing t on error.
func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// requireError fails t unless err is non-nil and its message contains want.
func requireError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}

// requireMissing fails t unless name does not exist beneath root.
func requireMissing(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
		t.Fatalf("%s: expected missing file, got %v", name, err)
	}
}

// symlink creates a test link, skipping t if symlinks are unavailable.
func symlink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// archiveEntry records a ZIP entry's contents, file mode, and modification time.
type archiveEntry struct {
	content  string
	mode     fs.FileMode
	modified time.Time
}

// readArchive reads ZIP entries indexed by archive path.
// Duplicate entry names fail the test.
func readArchive(t *testing.T, filename string) map[string]archiveEntry {
	t.Helper()
	archive, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := make(map[string]archiveEntry)
	for _, f := range archive.File {
		if _, exists := entries[f.Name]; exists {
			t.Fatalf("duplicate ZIP entry %q", f.Name)
		}
		reader, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		entries[f.Name] = archiveEntry{content: string(content), mode: f.Mode(), modified: f.Modified}
	}
	return entries
}

// failingWriter rejects writes with a supplied error.
type failingWriter struct {
	err error
}

// Write returns the configured error without accepting data.
func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// outputRecorder delivers written messages through a channel.
type outputRecorder struct {
	events chan string
}

// Write records p without blocking, or returns an error if the channel is full.
func (w *outputRecorder) Write(p []byte) (int, error) {
	select {
	case w.events <- string(p):
		return len(p), nil
	default:
		return 0, fmt.Errorf("output buffer full")
	}
}

// waitForOutput waits for matching output and, if provided, a ready condition.
// It fails t if no such notification arrives before the timeout.
func waitForOutput(t *testing.T, events <-chan string, want string, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	var received []string
	for {
		select {
		case message := <-events:
			received = append(received, message)
			if strings.Contains(message, want) && (ready == nil || ready()) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q; output: %q", want, received)
		}
	}
}
