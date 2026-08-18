package main

import (
	"bytes"
	"context"
	"errors"
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

// TestWorktreeCreateAcceptsAnyBranchName confirms the explicit form removes the
// ambiguity: once "create" is given, a branch may be named anything, including
// something that matches a subcommand.
func TestWorktreeCreateAcceptsAnyBranchName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"dev/24x/feature",
		"feature",
		"list",
		"help",
		"remove",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveBranchArg([]string{name})
			if err != nil {
				t.Errorf("resolveBranchArg(%q) returned %v, want it accepted", name, err)
			}

			if got != name {
				t.Errorf("resolveBranchArg(%q) = %q", name, got)
			}
		})
	}
}

func TestWorktreeCreateRequiresABranch(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {}, {""}, {"   "}} {
		if _, err := resolveBranchArg(args); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("resolveBranchArg(%v) = %v, want a rejection", args, err)
		}
	}
}
