package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	// lockRetryInterval is the delay between registry lock attempts.
	lockRetryInterval = 25 * time.Millisecond
	// lockTimeout limits how long a registry lock attempt waits.
	lockTimeout = 5 * time.Second
	// lockStaleAfter is the age after which a lock may be reclaimed.
	lockStaleAfter = 30 * time.Second
)

// errRegistryCorrupt indicates the registry file exists but cannot be parsed.
var errRegistryCorrupt = errors.New("worktree registry is corrupt")

// reservationTimeout is how long a pending reservation survives pruning
// without its worktree appearing. Creating the checkout takes seconds, so a
// reservation this old belongs to a creation that was interrupted.
const reservationTimeout = time.Hour

// errNameReserved indicates another entry already holds a worktree path or
// instance name.
var errNameReserved = errors.New("worktree name is already reserved")

// entry records the source checkout and branch, resolved worktree path, branch,
// Docker instance, cloning state, creation time, and pending reservation state.
// add clears a pending reservation when the worktree exists.
type entry struct {
	SourcePath string `json:"source_path"`

	SourceBranch string `json:"source_branch"`

	WorktreePath string `json:"worktree_path"`

	Branch string `json:"branch"`

	Instance string `json:"instance"`

	Cloned bool `json:"cloned"`

	CreatedAt time.Time `json:"created_at"`

	Pending bool `json:"pending,omitempty"`
}

// registry is the on-disk record of worktrees xf has created.
//
// It exists so that worktrees can be listed across projects, which git alone
// cannot do. It is deliberately *not* the source of truth: git and Docker are.
// Worktrees can be removed behind xf's back, so callers must reconcile entries
// against reality rather than trusting the file.
//
// A missing registry reads as empty. add and remove replace a corrupt registry,
// but reserve, listing, and pruning report corruption instead of treating
// unknown entries as absent. Read failures are never treated as empty.
type registry struct {
	mu   sync.Mutex
	path string
}

// newRegistry opens the registry in the user's configuration directory.
func newRegistry() (*registry, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}

	return &registry{path: path}, nil
}

// registryPath returns where the registry is stored, so callers can name the
// file when telling the user how to deal with a damaged one.
func registryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user config directory: %w", err)
	}

	return filepath.Join(dir, "xf", "worktrees.json"), nil
}

// all returns every recorded entry.
//
// A missing registry returns no entries and no error.
func (r *registry) all() ([]entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.load()
}

// forSource returns the entries belonging to a single checkout.
func (r *registry) forSource(sourcePath string) ([]entry, error) {
	entries, err := r.all()
	if err != nil {
		return nil, err
	}

	want := resolveSymlinks(sourcePath)

	var matched []entry

	for _, e := range entries {
		if resolveSymlinks(e.SourcePath) == want {
			matched = append(matched, e)
		}
	}

	return matched, nil
}

// add records a worktree, replacing any existing entry for the same path.
// It replaces a corrupt registry but preserves it on other read failures.
func (r *registry) add(e entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	unlock, err := r.lock()
	if err != nil {
		return err
	}

	defer unlock()

	entries, err := r.load()
	if err != nil && !errors.Is(err, errRegistryCorrupt) {
		return err
	}

	want := resolveSymlinks(e.WorktreePath)
	same := func(x entry) bool { return resolveSymlinks(x.WorktreePath) == want }

	i := slices.IndexFunc(entries, same)
	if i < 0 {
		return r.save(append(entries, e))
	}

	entries[i] = e

	return r.save(append(entries[:i+1], slices.DeleteFunc(entries[i+1:], same)...))
}

// reserve records entry before its worktree exists, unless another entry
// already holds its path or instance name.
//
// The check and the write happen under the registry lock, so two concurrent
// creations cannot both claim a name: the second sees the first's entry and
// gets errNameReserved. The caller replaces the entry with add once the
// worktree exists, or drops it with remove if creation fails.
//
// Unlike add, a corrupt registry is an error here. Its entries are what keep
// names unique, so an unreadable one cannot be treated as empty.
func (r *registry) reserve(e entry) error {
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

	want := filepath.Clean(e.WorktreePath)

	if slices.ContainsFunc(entries, func(x entry) bool {
		return filepath.Clean(x.WorktreePath) == want || x.Instance == e.Instance
	}) {
		return fmt.Errorf("%w: %s", errNameReserved, e.Instance)
	}

	e.Pending = true

	return r.save(append(entries, e))
}

// prune drops entries whose worktree directory no longer exists and returns
// how many were dropped.
//
// A pending reservation has no directory yet by design, so it is kept until
// reservationTimeout has passed. The check runs under the registry lock, so a
// reservation made while pruning is either seen as pending or not seen at all,
// never dropped as stale. Only confirmed missing paths are removed; permission
// and I/O errors do not establish absence.
func (r *registry) prune(now time.Time) (int, error) {
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

	kept := make([]entry, 0, len(entries))

	for _, e := range entries {
		if e.Pending && now.Sub(e.CreatedAt) < reservationTimeout {
			kept = append(kept, e)

			continue
		}

		if _, err := os.Stat(e.WorktreePath); errors.Is(err, fs.ErrNotExist) {
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

// remove drops the entry for a worktree path. Removing an absent entry is not
// an error, so cleanup is idempotent. It replaces a corrupt registry but
// preserves it on other read failures.
func (r *registry) remove(worktreePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	unlock, err := r.lock()
	if err != nil {
		return err
	}

	defer unlock()

	entries, err := r.load()
	if err != nil && !errors.Is(err, errRegistryCorrupt) {
		return err
	}

	want := resolveSymlinks(worktreePath)
	kept := slices.DeleteFunc(entries, func(e entry) bool { return resolveSymlinks(e.WorktreePath) == want })

	return r.save(kept)
}

// lockPath returns the path of the cross-process lockfile guarding r.path.
func (r *registry) lockPath() string {
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
// Stale locks are reclaimed by atomic rename. Release also renames the lock
// before checking its token, restoring it if it belongs to another process.
// Failed takeover attempts wait until the next retry or the deadline.
func (r *registry) lock() (func(), error) {
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
				release := fmt.Sprintf("%s.release.%d", path, os.Getpid())
				if err := os.Rename(path, release); err != nil {
					return
				}

				held, readErr := os.ReadFile(release)
				if readErr == nil && string(held) == token {
					_ = os.Remove(release)

					return
				}

				_ = os.Rename(release, path)
			}, nil
		}

		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("failed to lock worktree registry: %w", err)
		}

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

// load reads the registry, treating a missing file as empty and reporting corruption.
func (r *registry) load() ([]entry, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to read worktree registry: %w", err)
	}

	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w at %s: %w", errRegistryCorrupt, r.path, err)
	}

	return entries, nil
}

// save writes the registry atomically, so a crash or a concurrent run cannot
// leave a half-written file.
func (r *registry) save(entries []entry) error {
	if entries == nil {
		entries = []entry{}
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
