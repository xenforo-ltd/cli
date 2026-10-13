package worktree

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/ui"
)

// CompletionRefs lists the branches and directory names of this project's
// worktrees, both of which worktree references accept. Entries are checked
// against the filesystem because a worktree can be deleted without telling the
// registry.
func CompletionRefs(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	source, err := managementSource(cmd.Context(), cwd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	registry, err := newRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	entries, err := registry.forSource(source)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	seen := make(map[string]bool, len(entries))
	completions := make([]cobra.Completion, 0, len(entries))

	add := func(ref, description string) {
		if ref == "" || seen[ref] {
			return
		}

		seen[ref] = true
		completions = append(completions, cobra.CompletionWithDesc(ref, description))
	}

	for _, entry := range entries {
		if entry.Pending {
			continue
		}

		if _, err := os.Stat(entry.WorktreePath); err != nil {
			continue
		}

		description := ui.ShortHome(entry.WorktreePath)
		add(entry.Branch, description)
		add(filepath.Base(entry.WorktreePath), description)
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
