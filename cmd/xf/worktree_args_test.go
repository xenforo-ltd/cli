package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestWorktreeParentTakesNoArguments is the guard against the original footgun:
// `xf worktree lst` silently created a branch called "lst" instead of reporting
// a mistyped subcommand. The parent dispatches subcommands only, so an
// unrecognised name is now an error rather than a new worktree.
func TestWorktreeParentTakesNoArguments(t *testing.T) {
	configureErrorHandling(rootCmd)

	var out bytes.Buffer

	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"worktree", "lst"})

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err := rootCmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected an unrecognised subcommand to be rejected")
	}

	if !strings.Contains(err.Error(), "lst") {
		t.Errorf("error %q does not name the unrecognised argument", err)
	}
}
