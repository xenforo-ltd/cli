package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// findCommand locates a command by its full path from the root, e.g.
// ("auth", "login"), so tests exercise the real command tree.
func findCommand(t *testing.T, path ...string) *cobra.Command {
	t.Helper()

	cmd, _, err := rootCmd.Find(path)
	if err != nil {
		t.Fatalf("could not find command %v: %v", path, err)
	}

	if cmd.Name() != path[len(path)-1] {
		t.Fatalf("resolved %v to %q, want %q", path, cmd.Name(), path[len(path)-1])
	}

	return cmd
}

// runTree executes the real command tree with error handling configured, so the
// usageError wrapper added in configureErrorHandling is genuinely exercised.
// Calling cmd.Args directly would bypass it.
func runTree(t *testing.T, args ...string) error {
	t.Helper()

	configureErrorHandling(rootCmd)

	var out bytes.Buffer

	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	_, err := rootCmd.ExecuteContextC(context.Background())

	return err
}

// TestLeafCommandsRejectUnexpectedArguments asserts the contract: a non-nil
// error naming the offending token, classified as a usage error so that usage
// is printed. The exact message is cobra's and is not asserted verbatim.
func TestLeafCommandsRejectUnexpectedArguments(t *testing.T) {
	leaves := [][]string{
		{"doctor"},
		{"version"},
		{"licenses"},
		{"download"},
		{"self-update"},
		{"cache", "list"},
		{"cache", "purge"},
		{"cache", "path"},
		{"auth", "login"},
		{"auth", "status"},
		{"auth", "logout"},
		{"auth", "refresh"},
	}

	for _, path := range leaves {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			cmd := findCommand(t, path...)

			if cmd.Args == nil {
				t.Fatal("command has no Args validator; expected cobra.NoArgs")
			}

			err := cmd.Args(cmd, []string{"unexpected-token"})
			if err == nil {
				t.Fatal("expected an error for an unexpected argument")
			}

			if !strings.Contains(err.Error(), "unexpected-token") {
				t.Errorf("error %q does not mention the offending argument", err)
			}
		})
	}
}

// TestParentCommandsRejectUnknownSubcommands covers the case that motivated the
// change: `xf auth bogus` previously printed the parent's help and exited 0.
//
// cobra.NoArgs alone is not sufficient on a parent with subcommands: without a
// RunE, cobra skips the parent's Args validator entirely. Both must be set.
func TestParentCommandsRejectUnknownSubcommands(t *testing.T) {
	for _, parent := range []string{"auth", "cache"} {
		t.Run(parent, func(t *testing.T) {
			err := runTree(t, parent, "bogus")
			if err == nil {
				t.Fatal("expected an error for an unknown subcommand")
			}

			if !strings.Contains(err.Error(), "bogus") {
				t.Errorf("error %q does not mention the unknown subcommand", err)
			}

			var usageErr *usageError
			if !errors.As(err, &usageErr) {
				t.Errorf("expected a usageError so usage is printed, got %T", err)
			}
		})
	}
}

// TestParentCommandsStillRunWithoutArguments ensures adding RunE did not change
// the behaviour of invoking a group command on its own: it prints help and
// succeeds.
func TestParentCommandsStillRunWithoutArguments(t *testing.T) {
	for _, parent := range []string{"auth", "cache"} {
		t.Run(parent, func(t *testing.T) {
			if err := runTree(t, parent); err != nil {
				t.Errorf("expected bare %q to succeed, got %v", parent, err)
			}
		})
	}
}

// TestKnownSubcommandsStillDispatch guards against the parent's Args validator
// intercepting valid subcommands.
func TestKnownSubcommandsStillDispatch(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"cache", "path"})
	if err != nil {
		t.Fatalf("failed to resolve cache path: %v", err)
	}

	if cmd.Name() != "path" {
		t.Errorf("cache path resolved to %q, want dispatch to the subcommand", cmd.Name())
	}
}
