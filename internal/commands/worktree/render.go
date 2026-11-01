package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/ui"
)

// worktreeOutput is the machine-readable form of a created worktree.
type worktreeOutput struct {
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	SourcePath   string    `json:"source_path"`
	SourceBranch string    `json:"source_branch"`
	Instance     string    `json:"instance"`
	Cloned       bool      `json:"cloned"`
	CreatedAt    time.Time `json:"created_at"`
}

// worktreeState reports whether a registered worktree still exists on disk.
//
// The value is unstyled so it can go straight into JSON; renderWorktreeState
// colours it for the table.
func worktreeState(worktreePath string) string {
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}

		// A permission or I/O failure says nothing about whether the worktree
		// is there. Reporting "ok" would claim it is usable on the strength of
		// a check that never completed.
		return "unknown"
	}

	return "ok"
}

// renderWorktreeState colours a worktree's state for display.
//
// Anything that is not a confirmed "ok" (including "unknown" from a failed
// stat) renders as a warning: the table must never claim a worktree is usable
// on the strength of a check that never completed.
func renderWorktreeState(state string) string {
	switch state {
	case "ok":
		return ui.Success.Render("Ready")
	case "missing":
		return ui.Warning.Render("Missing")
	default:
		return ui.Warning.Render("Unknown")
	}
}

// printWorktrees renders entries, reconciling them against the filesystem.
//
// The registry is a record, not the source of truth: worktrees get removed
// outside xf, so entries are checked rather than trusted.
func printWorktrees(entries []entry, jsonOutput bool) error {
	if jsonOutput {
		// The same reconciliation the table performs, so machine consumers
		// are not told about worktrees that no longer exist on disk.
		type worktreeListEntry struct {
			entry

			State string `json:"state"`
		}

		listed := make([]worktreeListEntry, 0, len(entries))
		for _, entry := range entries {
			listed = append(listed, worktreeListEntry{
				entry: entry,
				State: worktreeState(entry.WorktreePath),
			})
		}

		return printJSON(listed)
	}

	if len(entries) == 0 {
		ui.PrintEmpty("No worktrees found")

		return nil
	}

	headers := []string{"BRANCH", "PATH", "INSTANCE", "STATE"}
	rows := make([][]string, 0, len(entries))

	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Branch,
			shortenPath(entry.WorktreePath),
			entry.Instance,
			renderWorktreeState(worktreeState(entry.WorktreePath)),
		})
	}

	ui.PrintInfo(ui.Plural(len(rows), "worktree", "worktrees"))
	ui.Println()
	ui.Println(ui.NewTable(headers, rows))

	return nil
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode output: %w", err)
	}

	ui.Println(string(data))

	return nil
}

// shortenPath replaces the home directory with ~ for display.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}

	return path
}
