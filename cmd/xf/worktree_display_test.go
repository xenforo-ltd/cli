package main

import (
	"testing"

	"github.com/xenforo-ltd/cli/internal/ui"
)

// TestRenderWorktreeState pins what the table claims about a worktree. An
// unverified stat (state "unknown") must never render as the green "Ready":
// the check did not complete, so readiness is not established.
func TestRenderWorktreeState(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		state string
		want  string
	}{
		{"ok", ui.Success.Render("Ready")},
		{"missing", ui.Warning.Render("Missing")},
		{"unknown", ui.Warning.Render("Unknown")},
		{"future-state", ui.Warning.Render("Unknown")},
	} {
		if got := renderWorktreeState(tc.state); got != tc.want {
			t.Errorf("renderWorktreeState(%q) = %q, want %q", tc.state, got, tc.want)
		}
	}
}
