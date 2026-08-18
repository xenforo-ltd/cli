package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyFileDoesNotWriteThroughASymlink covers the re-clone case: a previous
// clone can leave a symlink at the destination, and following it would write
// into whatever it points at, including a file in the source tree.
func TestCopyFileDoesNotWriteThroughASymlink(t *testing.T) {
	dir := t.TempDir()

	precious := filepath.Join(dir, "precious.txt")
	if err := os.WriteFile(precious, []byte("original contents"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst.txt")
	if err := os.Symlink(precious, dst); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst, 0o600); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(precious)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "original contents" {
		t.Errorf("wrote through the symlink and clobbered the target: %q", got)
	}
}
