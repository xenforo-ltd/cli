package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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

	// A damaged registry must not block recording new work, so parse failures
	// are treated as an empty registry and overwritten.
	entries, _ := r.load()

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

	entries, err := r.load()
	if err != nil {
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
		return nil, fmt.Errorf("failed to parse worktree registry at %s: %w", r.path, err)
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
