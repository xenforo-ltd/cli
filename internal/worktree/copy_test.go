package worktree

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const windowsOS = "windows"

func TestCopyTreeCopiesFilesAndDirectories(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	mustWrite(t, filepath.Join(src, "top.txt"), "top")
	mustWrite(t, filepath.Join(src, "nested", "deep", "file.txt"), "deep")

	if err := CopyTree(t.Context(), src, dst, nil); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	assertContent(t, filepath.Join(dst, "top.txt"), "top")
	assertContent(t, filepath.Join(dst, "nested", "deep", "file.txt"), "deep")
}

// TestCopyTreePreservesModes matters because XenForo checks that data/ and
// internal_data/ are writable.
//
// The umask is set strictly here because relying on the ambient umask made
// this assertion pass only by luck: a permissive umask (022 or looser) never
// exercises the case CopyTree exists to handle, where OpenFile's requested
// mode gets bits cleared away by the umask at creation time.
func TestCopyTreePreservesModes(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("file mode bits are not meaningful on Windows")
	}

	old := setUmask(0o077)
	defer setUmask(old)

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	path := filepath.Join(src, "script.sh")
	mustWrite(t, path, "#!/bin/sh")

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := CopyTree(t.Context(), src, dst, nil); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	info, err := os.Stat(filepath.Join(dst, "script.sh"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestCopyTreeReportsProgress(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		mustWrite(t, filepath.Join(src, name), name)
	}

	var seen int

	err := CopyTree(t.Context(), src, dst, func(copied, total int) {
		seen = copied

		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
	})
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	if seen != 3 {
		t.Errorf("final progress = %d, want 3", seen)
	}
}

// TestCopyTreeSkipsMissingSource covers a source directory that does not exist,
// which is normal: a XenForo install may have no data/ yet.
func TestCopyTreeSkipsMissingSource(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "target")

	if err := CopyTree(t.Context(), filepath.Join(t.TempDir(), "absent"), dst, nil); err != nil {
		t.Errorf("a missing source must not be an error, got %v", err)
	}
}

func TestCopyTreeIsCancellable(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	for i := range 50 {
		mustWrite(t, filepath.Join(src, string(rune('a'+i%26))+string(rune('0'+i/26))+".txt"), "x")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := CopyTree(ctx, src, dst, nil); err == nil {
		t.Error("expected a cancelled copy to return an error")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if string(data) != want {
		t.Errorf("%s = %q, want %q", path, data, want)
	}
}

// A source directory without its owner-write bit must not stop the copy: the
// mode is applied after the directory's contents are in place, not before.
func TestCopyTreeCopiesIntoReadOnlyDirectories(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("file mode bits are not meaningful on Windows")
	}

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "target")

	readOnly := filepath.Join(src, "locked")
	if err := os.Mkdir(readOnly, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mustWrite(t, filepath.Join(readOnly, "data.txt"), "payload")

	if err := os.Chmod(readOnly, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() {
		// Restore write access so the temp directory can be removed.
		_ = os.Chmod(readOnly, 0o755)
		_ = os.Chmod(filepath.Join(dst, "locked"), 0o755)
	})

	if err := CopyTree(t.Context(), src, dst, nil); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dst, "locked", "data.txt"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}

	if string(content) != "payload" {
		t.Errorf("content = %q, want %q", content, "payload")
	}

	info, err := os.Stat(filepath.Join(dst, "locked"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Mode().Perm() != 0o555 {
		t.Errorf("directory mode = %o, want 555", info.Mode().Perm())
	}
}
