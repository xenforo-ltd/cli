package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

	// Reading a damaged registry must report it rather than pass off an empty
	// list as the truth: the two mean very different things to the user.
	entries, err := reg.All()
	if !errors.Is(err, ErrRegistryCorrupt) {
		t.Fatalf("All over a corrupt registry: got %v, want ErrRegistryCorrupt", err)
	}

	if entries != nil {
		t.Errorf("All returned %d entries alongside an error, want none", len(entries))
	}

	// Adding must still succeed, replacing the damaged file.
	if err := reg.Add(Entry{Branch: "x", WorktreePath: "/tmp/x"}); err != nil {
		t.Fatalf("Add over a corrupt registry: %v", err)
	}

	entries, err = reg.All()
	if err != nil {
		t.Fatalf("All after recovery: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 after recovery", len(entries))
	}
}

// TestRegistryRemoveOverCorruptFileIsNotFatal covers the same tolerance as
// TestRegistryCorruptFileIsNotFatal, but for Remove: cleanup during
// "xf worktree remove" or prune must not fail just because the registry is
// unreadable.
func TestRegistryRemoveOverCorruptFileIsNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg := &Registry{path: path}

	if err := reg.Remove("/tmp/anything"); err != nil {
		t.Fatalf("Remove over a corrupt registry: %v", err)
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

// TestRegistryAddSurvivesConcurrentProcesses exercises the cross-process lock:
// separate Registry values sharing one file stand in for separate "xf
// worktree" invocations, which previously could read-modify-write the same
// JSON concurrently and lose each other's entries.
func TestRegistryAddSurvivesConcurrentProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")

	const writers = 8

	var wg sync.WaitGroup

	for i := range writers {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			reg := &Registry{path: path}
			entry := Entry{WorktreePath: fmt.Sprintf("/tmp/wt-%d", i), Branch: fmt.Sprintf("b%d", i)}

			if err := reg.Add(entry); err != nil {
				t.Errorf("Add from writer %d: %v", i, err)
			}
		}(i)
	}

	wg.Wait()

	reg := &Registry{path: path}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(entries) != writers {
		t.Errorf("got %d entries, want %d: entries were lost to a race", len(entries), writers)
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

// A registry that cannot be read is not an empty registry. Treating it as one
// would let the following save discard every entry it failed to read.
func TestRegistryDoesNotDiscardEntriesItCannotRead(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("unreadable-file permissions are not enforced the same way on Windows")
	}

	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "worktrees.json")

	reg := &Registry{path: path}

	if err := reg.Add(Entry{WorktreePath: "/src.worktrees/keep", Branch: "keep"}); err != nil {
		t.Fatalf("seed Add: %v", err)
	}

	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	if err := reg.Add(Entry{WorktreePath: "/src.worktrees/new", Branch: "new"}); err == nil {
		t.Error("Add over an unreadable registry should fail rather than overwrite it")
	}

	if err := reg.Remove("/src.worktrees/keep"); err == nil {
		t.Error("Remove over an unreadable registry should fail rather than overwrite it")
	}

	// The original entry must still be there once the file is readable again.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod back: %v", err)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(entries) != 1 || entries[0].Branch != "keep" {
		t.Errorf("entries = %+v, want the seeded entry preserved", entries)
	}
}

// TestRegistryReserveRejectsAClaimedName covers the reservation that keeps two
// concurrent creations from settling on the same worktree or instance name.
func TestRegistryReserveRejectsAClaimedName(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	first := Entry{WorktreePath: "/s/main.worktrees/feature", Instance: "feature"}
	if err := reg.Reserve(first); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	for _, clash := range []Entry{
		{WorktreePath: "/s/main.worktrees/feature", Instance: "other"},
		{WorktreePath: "/t/main.worktrees/feature", Instance: "feature"},
	} {
		if err := reg.Reserve(clash); !errors.Is(err, ErrNameReserved) {
			t.Errorf("Reserve(%+v) = %v, want ErrNameReserved", clash, err)
		}
	}

	if err := reg.Reserve(Entry{WorktreePath: "/s/main.worktrees/feature-2", Instance: "feature-2"}); err != nil {
		t.Errorf("Reserve of a free name: %v", err)
	}
}

// TestRegistryReserveConcurrently is the race the reservation exists for:
// however many processes reserve the same name at once, exactly one wins.
func TestRegistryReserveConcurrently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")

	const workers = 8

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)

	for i := range workers {

		wg.Go(func() {

			// Separate Registry values, as separate processes would have.
			reg := &Registry{path: path}

			err := reg.Reserve(Entry{WorktreePath: fmt.Sprintf("/src%d/main.worktrees/feature", i), Instance: "feature"})
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			} else if !errors.Is(err, ErrNameReserved) {
				t.Errorf("Reserve: %v", err)
			}
		})
	}

	wg.Wait()

	if wins != 1 {
		t.Errorf("%d reservations of the same name succeeded, want exactly 1", wins)
	}
}

// TestRegistryReserveRefusesACorruptRegistry covers the difference from Add:
// the entries are what keep names unique, so an unreadable registry must stop
// allocation rather than read as empty.
func TestRegistryReserveRefusesACorruptRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg := &Registry{path: path}

	if err := reg.Reserve(Entry{WorktreePath: "/s/main.worktrees/feature", Instance: "feature"}); !errors.Is(err, ErrRegistryCorrupt) {
		t.Errorf("Reserve = %v, want ErrRegistryCorrupt", err)
	}
}

// TestRegistryPruneKeepsActiveReservations covers prune running while another
// process is creating a worktree: its reservation has no directory yet but
// must survive, or another checkout could claim the same instance name.
func TestRegistryPruneKeepsActiveReservations(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}
	dir := t.TempDir()
	now := time.Now().UTC()

	existing := filepath.Join(dir, "existing")
	if err := os.Mkdir(existing, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := reg.Add(Entry{WorktreePath: existing, Instance: "existing", CreatedAt: now}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := reg.Add(Entry{WorktreePath: filepath.Join(dir, "stale"), Instance: "stale", CreatedAt: now}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := reg.Reserve(Entry{WorktreePath: filepath.Join(dir, "creating"), Instance: "creating", CreatedAt: now}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	if err := reg.Reserve(Entry{
		WorktreePath: filepath.Join(dir, "abandoned"),
		Instance:     "abandoned",
		CreatedAt:    now.Add(-2 * ReservationTimeout),
	}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	pruned, err := reg.Prune(now)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if pruned != 2 {
		t.Errorf("pruned %d entries, want 2 (stale and abandoned)", pruned)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	var kept []string
	for _, e := range entries {
		kept = append(kept, e.Instance)
	}

	if len(kept) != 2 || kept[0] != "existing" || kept[1] != "creating" {
		t.Errorf("kept %v, want [existing creating]", kept)
	}
}

// TestRegistryAddClearsPending covers a reservation becoming an ordinary entry
// once its worktree exists.
func TestRegistryAddClearsPending(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}
	entry := Entry{WorktreePath: "/s/main.worktrees/feature", Instance: "feature"}

	if err := reg.Reserve(entry); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	if err := reg.Add(entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	entries, err := reg.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(entries) != 1 || entries[0].Pending {
		t.Errorf("entries = %+v, want one entry that is no longer pending", entries)
	}
}

// TestForWorktreeReturnsTheRecordedEntry covers the lookup that makes an
// adopted checkout addressable at a non-conventional location.
func TestForWorktreeReturnsTheRecordedEntry(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	entry := Entry{
		SourcePath:   "/Users/x/Sites/main",
		SourceBranch: "main",
		WorktreePath: "/Users/x/Sites/elsewhere",
		Branch:       "feature",
		Instance:     "feature",
	}

	if err := reg.Add(entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok, err := reg.ForWorktree(entry.WorktreePath)
	if err != nil || !ok {
		t.Fatalf("ForWorktree = %+v, %v, %v; want a match", got, ok, err)
	}

	if got.Branch != "feature" || got.SourcePath != entry.SourcePath {
		t.Errorf("ForWorktree = %+v, want the recorded entry", got)
	}
}

// TestForWorktreeIsNotAnErrorWhenAbsent covers the registry being a discovery
// aid: no test for a path is simply no match.
func TestForWorktreeIsNotAnErrorWhenAbsent(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	if _, ok, err := reg.ForWorktree("/Users/x/Sites/absent"); err != nil || ok {
		t.Fatalf("ForWorktree = ok %v, err %v; want no match and no error", ok, err)
	}
}

// TestForWorktreeMatchesThroughASymlink covers path identity: an alias and the
// directory it resolves to name the same record.
func TestForWorktreeMatchesThroughASymlink(t *testing.T) {
	dir := t.TempDir()

	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	reg := &Registry{path: filepath.Join(dir, "worktrees.json")}

	if err := reg.Add(Entry{SourcePath: "/s/main", WorktreePath: real, Branch: "feature"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok, err := reg.ForWorktree(alias)
	if err != nil || !ok {
		t.Fatalf("ForWorktree(alias) = %+v, %v, %v; want a match", got, ok, err)
	}

	if got.WorktreePath != real {
		t.Errorf("WorktreePath = %q, want %q", got.WorktreePath, real)
	}
}

// TestForWorktreeReportsConflictingSources covers the ambiguity that a
// hand-edited or merged registry can hold. Add collapses equivalent paths, so
// the pair is written directly.
func TestForWorktreeReportsConflictingSources(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	checkout := filepath.Join(t.TempDir(), "checkout")

	if err := reg.save([]Entry{
		{SourcePath: "/Users/x/Sites/main", WorktreePath: checkout, Branch: "feature"},
		{SourcePath: "/Users/x/Sites/other", WorktreePath: checkout, Branch: "feature"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, _, err := reg.ForWorktree(checkout); !errors.Is(err, ErrRegistryAmbiguous) {
		t.Fatalf("ForWorktree = %v, want ErrRegistryAmbiguous", err)
	}
}

// TestForWorktreeToleratesAgreeingDuplicates covers an older xf leaving the
// same record twice: equivalent records that agree about the source are one
// record, not an ambiguity.
func TestForWorktreeToleratesAgreeingDuplicates(t *testing.T) {
	reg := &Registry{path: filepath.Join(t.TempDir(), "worktrees.json")}

	checkout := filepath.Join(t.TempDir(), "checkout")

	if err := reg.save([]Entry{
		{SourcePath: "/Users/x/Sites/main", SourceBranch: "main", WorktreePath: checkout, Branch: "feature"},
		{SourcePath: "/Users/x/Sites/main", SourceBranch: "other", WorktreePath: checkout, Branch: "feature"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok, err := reg.ForWorktree(checkout)
	if err != nil || !ok {
		t.Fatalf("ForWorktree = %+v, %v, %v; want a single match", got, ok, err)
	}

	if got.SourceBranch != "main" {
		t.Errorf("SourceBranch = %q, want the first agreeing record", got.SourceBranch)
	}
}
