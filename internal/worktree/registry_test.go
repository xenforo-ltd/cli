package worktree

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")

	reg := &Registry{path: path}

	entry := Entry{
		SourcePath:   "/Users/x/Sites/main",
		SourceBranch: "main",
		WorktreePath: "/Users/x/Sites/main.worktrees/dev-24x-feature",
		Branch:       "dev/24x/feature",
		Instance:     "dev-24x-feature",
		Cloned:       true,
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := reg.Add(entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	got := entries[0]
	if got.Branch != entry.Branch || got.Instance != entry.Instance || !got.Cloned {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

// TestRegistryMissingFileIsNotAnError covers the design rule that the registry
// is never load-bearing: a missing file yields no entries, not a failure.
func TestRegistryMissingFileIsNotAnError(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "absent.json")}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("a missing registry must not be an error, got %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

// TestRegistryCorruptFileIsNotFatal covers the same rule for damaged content.
func TestRegistryCorruptFileIsNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg := &Registry{path: path}

	if _, err := reg.All(); err == nil {
		t.Log("corrupt registry returned no error; acceptable if entries are empty")
	}

	// Adding must still succeed, replacing the damaged file.
	if err := reg.Add(Entry{Branch: "x", WorktreePath: "/tmp/x"}); err != nil {
		t.Fatalf("Add over a corrupt registry: %v", err)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All after recovery: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 after recovery", len(entries))
	}
}

func TestRegistryRemove(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	for _, p := range []string{"/tmp/a", "/tmp/b"} {
		if err := reg.Add(Entry{WorktreePath: p, Branch: filepath.Base(p)}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	if err := reg.Remove("/tmp/a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(entries) != 1 || entries[0].WorktreePath != "/tmp/b" {
		t.Errorf("unexpected entries after removal: %+v", entries)
	}
}

// TestRegistryAddIsIdempotent ensures re-registering the same path updates the
// entry rather than duplicating it.
func TestRegistryAddIsIdempotent(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	first := Entry{WorktreePath: "/tmp/a", Branch: "old", Instance: "one"}
	second := Entry{WorktreePath: "/tmp/a", Branch: "new", Instance: "two"}

	if err := reg.Add(first); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := reg.Add(second); err != nil {
		t.Fatalf("Add again: %v", err)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	if entries[0].Branch != "new" || entries[0].Instance != "two" {
		t.Errorf("entry was not updated: %+v", entries[0])
	}
}

func TestRegistryForSource(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	add := func(source, branch string) {
		t.Helper()

		if err := reg.Add(Entry{
			SourcePath:   source,
			Branch:       branch,
			WorktreePath: filepath.Join(source+".worktrees", branch),
		}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	add("/Users/x/Sites/main", "one")
	add("/Users/x/Sites/main", "two")
	add("/Users/x/Sites/other", "three")

	entries, err := reg.ForSource("/Users/x/Sites/main")
	if err != nil {
		t.Fatalf("ForSource: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("got %d entries for the source, want 2", len(entries))
	}
}
