package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// lockRetryInterval and lockTimeout bound how long a mutating call waits for
// another process to release the registry lock, so a crashed process cannot
// wedge every other xf invocation forever.
const (
	lockRetryInterval = 25 * time.Millisecond
	lockTimeout       = 5 * time.Second
	lockStaleAfter    = 30 * time.Second
)

// ErrRegistryCorrupt indicates the registry file exists but cannot be parsed.
//
// Mutating calls tolerate this and rebuild from an empty registry, so a damaged
// file never blocks cleanup. Read failures are not tolerated: they mean the
// existing entries are unknown rather than absent.
var ErrRegistryCorrupt = errors.New("worktree registry is corrupt")

// ReservationTimeout is how long a pending reservation survives pruning
// without its worktree appearing. Creating the checkout takes seconds, so a
// reservation this old belongs to a creation that was interrupted.
const ReservationTimeout = time.Hour

// ErrNameReserved indicates another entry already holds a worktree path or
// instance name.
var ErrNameReserved = errors.New("worktree name is already reserved")

// ErrRegistryAmbiguous indicates the registry holds conflicting records for one
// checkout path: several equivalent paths disagree about the source they
// belong to, so no source can be chosen without guessing.
var ErrRegistryAmbiguous = errors.New("worktree registry has conflicting entries")

// Entry records a worktree created or adopted by xf.
//
// A created record comes from "worktree create"; an adopted one from "worktree
// adopt", which registers a checkout xf did not make. Both describe the same
// fields, but an adopted entry claims nothing about how the checkout was
// produced: Cloned is false and CreatedAt is the registration time. An entry
// is metadata, never proof that a checkout is safe to remove.
type Entry struct {
	// SourcePath is the checkout that manages the worktree.
	SourcePath string `json:"source_path"`

	// SourceBranch is the branch or workspace the source had at registration.
	SourceBranch string `json:"source_branch"`

	// WorktreePath is the resolved location of the worktree.
	WorktreePath string `json:"worktree_path"`

	// Branch is the branch or workspace name checked out in the worktree.
	Branch string `json:"branch"`

	// Instance is the Docker instance name recorded for the worktree. For an
	// adopted worktree this is metadata: adoption does not rewrite .env or
	// retarget containers.
	Instance string `json:"instance"`

	// Cloned records whether the environment was cloned from the source. An
	// adopted entry is false: adoption never infers or claims a clone.
	Cloned bool `json:"cloned"`

	// CreatedAt is when the entry was created or last adopted.
	CreatedAt time.Time `json:"created_at"`

	// Pending marks a reservation for a worktree that is still being created.
	// Add clears it by replacing the entry once the worktree exists.
	Pending bool `json:"pending,omitempty"`
}

// Registry is the on-disk record of worktrees xf has created or adopted.
//
// It exists so that worktrees can be listed across projects, which git alone
// cannot do. It is deliberately *not* the source of truth: git and Docker are.
// Worktrees can be removed behind xf's back, so callers must reconcile entries
// against reality rather than trusting the file.
//
// It is also metadata rather than authority: a record, adopted or created, is
// never proof that a checkout is safe to remove. Removal validates the actual
// checkout and its repository before deleting anything.
//
// A missing registry reads as empty, and the operations that rewrite it --
// Add and Remove -- recover from a damaged one by replacing it, so creating
// and removing worktrees never fail on registry damage. Reading is the
// exception: All reports a corrupt file rather than silently presenting an
// empty list, because "no worktrees" and "the record is unreadable" call for
// different responses from the user. Listing and pruning therefore surface the
// error and say what to do about it.
type Registry struct {
	mu   sync.Mutex
	path string
}

// NewRegistry opens the registry in the user's configuration directory.
func NewRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}

	return &Registry{path: path}, nil
}

// RegistryPath returns where the registry is stored, so callers can name the
// file when telling the user how to deal with a damaged one.
func RegistryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user config directory: %w", err)
	}

	return filepath.Join(dir, "xf", "worktrees.json"), nil
}

// All returns every recorded entry.
//
// A missing registry returns no entries and no error.
func (r *Registry) All() ([]Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.load()
}

// ForSource returns the entries belonging to a single checkout.
//
// Paths are compared by their canonical identity, so an alias and the path it
// resolves to name the same source.
func (r *Registry) ForSource(sourcePath string) ([]Entry, error) {
	entries, err := r.All()
	if err != nil {
		return nil, err
	}

	want := resolveSymlinks(sourcePath)

	var matched []Entry

	for _, e := range entries {
		if resolveSymlinks(e.SourcePath) == want {
			matched = append(matched, e)
		}
	}

	return matched, nil
}

// ForWorktree returns the entry recorded for a checkout path, by the same
// canonical identity used everywhere else.
//
// A missing registry, or one with no record for the path, reports no match and
// no error. Equivalent legacy records are tolerated when they agree about the
// source, but conflicting sources are an ErrRegistryAmbiguous rather than an
// arbitrary choice between them.
func (r *Registry) ForWorktree(path string) (Entry, bool, error) {
	entries, err := r.All()
	if err != nil {
		return Entry{}, false, err
	}

	want := resolveSymlinks(path)

	var (
		match       Entry
		found       bool
		matchSource string
	)

	for _, e := range entries {
		if resolveSymlinks(e.WorktreePath) != want {
			continue
		}

		if !found {
			match = e
			matchSource = resolveSymlinks(e.SourcePath)
			found = true

			continue
		}

		if resolveSymlinks(e.SourcePath) != matchSource {
			return Entry{}, false, fmt.Errorf(
				"%w: %s is recorded with more than one source",
				ErrRegistryAmbiguous, path,
			)
		}
	}

	return match, found, nil
}

// Add records a worktree, replacing any existing entry for the same path.
func (r *Registry) Add(entry Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// A damaged registry must not block recording new work, so parse failures
	// are treated as an empty registry and overwritten. A read failure is
	// different: the entries are unreadable rather than absent, and saving
	// over them would discard every other worktree's record.
	entries, err := r.load()
	if err != nil && !errors.Is(err, ErrRegistryCorrupt) {
		return err
	}

	want := resolveSymlinks(entry.WorktreePath)

	// Equivalent records are collapsed into one updated entry, so a path
	// reached through both a symlink and its target, or left twice by an older
	// xf, ends up recorded once. The updated entry takes the first matching
	// slot to keep unrelated entries in their existing order.
	updated := make([]Entry, 0, len(entries)+1)

	replaced := false

	for _, e := range entries {
		if resolveSymlinks(e.WorktreePath) == want {
			if !replaced {
				updated = append(updated, entry)
				replaced = true
			}

			continue
		}

		updated = append(updated, e)
	}

	if !replaced {
		updated = append(updated, entry)
	}

	return r.save(updated)
}

// Reserve records entry before its worktree exists, unless another entry
// already holds its path or instance name.
//
// The check and the write happen under the registry lock, so two concurrent
// creations cannot both claim a name: the second sees the first's entry and
// gets ErrNameReserved. The caller replaces the entry with Add once the
// worktree exists, or drops it with Remove if creation fails.
//
// Unlike Add, a corrupt registry is an error here. Its entries are what keep
// names unique, so an unreadable one cannot be treated as empty.
func (r *Registry) Reserve(entry Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()

	entries, err := r.load()
	if err != nil {
		return err
	}

	want := filepath.Clean(entry.WorktreePath)

	for _, e := range entries {
		if filepath.Clean(e.WorktreePath) == want || e.Instance == entry.Instance {
			return fmt.Errorf("%w: %s", ErrNameReserved, entry.Instance)
		}
	}

	entry.Pending = true

	return r.save(append(entries, entry))
}

// Prune drops entries whose worktree directory no longer exists and returns
// how many were dropped.
//
// A pending reservation has no directory yet by design, so it is kept until
// ReservationTimeout has passed. The check runs under the registry lock, so a
// reservation made while pruning is either seen as pending or not seen at all,
// never dropped as stale.
func (r *Registry) Prune(now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	unlock, err := r.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

	entries, err := r.load()
	if err != nil {
		return 0, err
	}

	kept := make([]Entry, 0, len(entries))

	for _, e := range entries {
		if e.Pending && now.Sub(e.CreatedAt) < ReservationTimeout {
			kept = append(kept, e)

			continue
		}

		// Only a confirmed absence is stale: a permission or I/O failure says
		// nothing about whether the worktree is there.
		if _, err := os.Stat(e.WorktreePath); errors.Is(err, os.ErrNotExist) {
			continue
		}

		kept = append(kept, e)
	}

	pruned := len(entries) - len(kept)
	if pruned == 0 {
		return 0, nil
	}

	return pruned, r.save(kept)
}

// Remove drops the entry for a worktree path. Removing an absent entry is not
// an error, so cleanup is idempotent.
func (r *Registry) Remove(worktreePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// A damaged registry must not block cleanup: worktree removal and prune
	// have to be able to proceed even when the file cannot be parsed, so a
	// parse failure is treated the same as an empty registry, as in Add.
	//
	// Only a parse failure. A permission or I/O error means the existing
	// entries could not be read at all, and saving over them would delete
	// every other worktree's record.
	entries, err := r.load()
	if err != nil && !errors.Is(err, ErrRegistryCorrupt) {
		return err
	}

	want := resolveSymlinks(worktreePath)
	kept := make([]Entry, 0, len(entries))

	for _, e := range entries {
		if resolveSymlinks(e.WorktreePath) != want {
			kept = append(kept, e)
		}
	}

	return r.save(kept)
}

// lockPath returns the path of the cross-process lockfile guarding r.path.
func (r *Registry) lockPath() string {
	return r.path + ".lock"
}

// lock acquires a cross-process lock covering a load/modify/save transaction
// and returns a function that releases it.
//
// r.mu only guards one process's own goroutines; separate "xf worktree"
// invocations are separate processes that would otherwise read, modify and
// write the same JSON file with no coordination, silently losing whichever
// write happened first. A plain O_CREATE|O_EXCL lockfile is used rather than
// syscall.Flock so the same code works on Windows, where xf also builds.
func (r *Registry) lock() (func(), error) {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create registry directory: %w", err)
	}

	path := r.lockPath()
	deadline := time.Now().Add(lockTimeout)

	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			token := fmt.Sprintf("%d.%d", os.Getpid(), time.Now().UnixNano())

			_, _ = io.WriteString(f, token)
			_ = f.Close()

			return func() {
				// Released by renaming to a private name and deleting that,
				// rather than reading the token and then removing the path:
				// between those two steps the lock could be taken over as
				// stale and recreated by another process, and the remove would
				// delete a lock that is now theirs.
				//
				// Rename is atomic, so at most one process moves this file. If
				// the content is not ours, it was taken over and is put back
				// untouched.
				release := fmt.Sprintf("%s.release.%d", path, os.Getpid())
				if err := os.Rename(path, release); err != nil {
					return
				}

				held, readErr := os.ReadFile(release)
				if readErr == nil && string(held) == token {
					_ = os.Remove(release)

					return
				}

				// Someone else's lock: restore it.
				_ = os.Rename(release, path)
			}, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("failed to lock worktree registry: %w", err)
		}

		// A lockfile left behind by a process that crashed before releasing it
		// would otherwise wedge every future call, so a lock older than
		// lockStaleAfter is treated as abandoned and cleared.
		//
		// The takeover renames rather than removes: rename is atomic, so of
		// several processes that all see the same stale lock, only the one
		// whose rename succeeds clears it. Removing directly is a
		// check-then-act race in which two processes can each delete the
		// other's fresh lock and both believe they hold it.
		//
		// A failed takeover falls through to the deadline check and the sleep
		// rather than retrying immediately: a lockfile that cannot be removed,
		// on a read-only parent for instance, would otherwise spin forever.
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			stale := fmt.Sprintf("%s.stale.%d", path, os.Getpid())
			if renameErr := os.Rename(path, stale); renameErr == nil {
				_ = os.Remove(stale)

				continue
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for worktree registry lock at %s", path)
		}

		time.Sleep(lockRetryInterval)
	}
}

func (r *Registry) load() ([]Entry, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to read worktree registry: %w", err)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w at %s: %w", ErrRegistryCorrupt, r.path, err)
	}

	return entries, nil
}

// save writes the registry atomically, so a crash or a concurrent run cannot
// leave a half-written file.
func (r *Registry) save(entries []Entry) error {
	if entries == nil {
		entries = []Entry{}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode worktree registry: %w", err)
	}

	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "worktrees-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary registry file: %w", err)
	}

	tmpName := tmp.Name()

	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("failed to write worktree registry: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close worktree registry: %w", err)
	}

	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("failed to set registry permissions: %w", err)
	}

	if err := os.Rename(tmpName, r.path); err != nil {
		return fmt.Errorf("failed to replace worktree registry: %w", err)
	}

	return nil
}
