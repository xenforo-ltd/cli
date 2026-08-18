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

// Entry records a worktree created by xf.
type Entry struct {
	// SourcePath is the checkout the worktree was created from.
	SourcePath string `json:"source_path"`

	// SourceBranch is the branch the source was on at creation time.
	SourceBranch string `json:"source_branch"`

	// WorktreePath is the resolved location of the worktree.
	WorktreePath string `json:"worktree_path"`

	// Branch is the branch checked out in the worktree.
	Branch string `json:"branch"`

	// Instance is the Docker instance name for the worktree.
	Instance string `json:"instance"`

	// Cloned records whether the environment was cloned from the source.
	Cloned bool `json:"cloned"`

	// CreatedAt is when the worktree was created.
	CreatedAt time.Time `json:"created_at"`
}

// Registry is the on-disk record of worktrees xf has created.
//
// It exists so that worktrees can be listed across projects, which git alone
// cannot do. It is deliberately *not* the source of truth: git and Docker are.
// Worktrees can be removed behind xf's back, so callers must reconcile entries
// against reality rather than trusting the file. A missing or damaged registry
// degrades listing across projects but never blocks an operation.
type Registry struct {
	mu   sync.Mutex
	path string
}

// NewRegistry opens the registry in the user's configuration directory.
func NewRegistry() (*Registry, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine user config directory: %w", err)
	}

	return &Registry{path: filepath.Join(dir, "xf", "worktrees.json")}, nil
}

// Path returns the registry file location.
func (r *Registry) Path() string {
	return r.path
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
func (r *Registry) ForSource(sourcePath string) ([]Entry, error) {
	entries, err := r.All()
	if err != nil {
		return nil, err
	}

	want := filepath.Clean(sourcePath)

	var matched []Entry

	for _, e := range entries {
		if filepath.Clean(e.SourcePath) == want {
			matched = append(matched, e)
		}
	}

	return matched, nil
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

	want := filepath.Clean(entry.WorktreePath)
	replaced := false

	for i, e := range entries {
		if filepath.Clean(e.WorktreePath) == want {
			entries[i] = entry
			replaced = true

			break
		}
	}

	if !replaced {
		entries = append(entries, entry)
	}

	return r.save(entries)
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

	want := filepath.Clean(worktreePath)
	kept := make([]Entry, 0, len(entries))

	for _, e := range entries {
		if filepath.Clean(e.WorktreePath) != want {
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
