package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/xenforo-ltd/cli/internal/ui"
)

// worktreeOutput is the machine-readable form of a created or adopted worktree.
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
// colours it for the table. Inspection failures other than absence return
// unknown rather than claiming the worktree is usable.
func worktreeState(worktreePath string) string {
	if _, err := os.Stat(worktreePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "missing"
		}

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
// outside xf, so entries are checked rather than trusted in both output formats.
func printWorktrees(entries []entry, jsonOutput bool) error {
	if jsonOutput {
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
			ui.ShortHome(entry.WorktreePath),
			entry.Instance,
			renderWorktreeState(worktreeState(entry.WorktreePath)),
		})
	}

	ui.PrintInfo(ui.Plural(len(rows), "worktree", "worktrees"))
	ui.Println()
	ui.Println(ui.NewTable(headers, rows))

	return nil
}

// printJSON writes an indented JSON document to the output stream.
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode output: %w", err)
	}

	ui.Println(string(data))

	return nil
}
