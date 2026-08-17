package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRuntimeErrorsDoNotPrintUsage guards against the confusing behaviour where
// a runtime failure (for example Docker not being available) caused cobra to
// print the command's usage block, implying the user's syntax was wrong.
func TestRuntimeErrorsDoNotPrintUsage(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "php [path] -- [args...]",
		Args: cobra.MinimumNArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("failed to run PHP command: docker exploded")
		},
	}
	configureErrorHandling(cmd)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"development/scripts/sync-versions.php"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected the runtime error to be returned")
	}

	if strings.Contains(out.String(), "Usage:") {
		t.Errorf("usage was printed for a runtime error:\n%s", out.String())
	}
}

// TestUsageErrorsStillPrintUsage ensures silencing runtime usage output does not
// also hide usage for genuine misuse, where it is the helpful response.
func TestUsageErrorsStillPrintUsage(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "exec [path] <service> <command> [args...]",
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	configureErrorHandling(cmd)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"only-one"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected an argument validation error")
	}

	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("argument errors must be marked as usage errors, got %T: %v", err, err)
	}
}

// TestRuntimeErrorsAreNotUsageErrors ensures runtime failures are not marked as
// usage errors, so no usage block is printed for them.
func TestRuntimeErrorsAreNotUsageErrors(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "php",
		Args: cobra.MinimumNArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("failed to run PHP command: docker unavailable")
		},
	}
	configureErrorHandling(cmd)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"script.php"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected the runtime error to be returned")
	}

	var usageErr *usageError
	if errors.As(err, &usageErr) {
		t.Error("runtime errors must not be marked as usage errors")
	}
}

// TestErrorsArePrintedOnlyOnce guards against cobra printing a returned error
// and Execute's own handler printing the same error again.
func TestErrorsArePrintedOnlyOnce(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use: "php",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("distinctive-failure-marker")
		},
	}
	configureErrorHandling(cmd)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)

	if _, err := cmd.ExecuteContextC(context.Background()); err == nil {
		t.Fatal("expected an error")
	}

	if got := strings.Count(out.String(), "distinctive-failure-marker"); got != 0 {
		t.Errorf("cobra printed the error itself %d time(s); Execute is responsible for printing:\n%s", got, out.String())
	}
}

// TestConfigureErrorHandlingAppliesToSubcommands ensures the behaviour is applied
// across the whole command tree rather than only the root.
func TestConfigureErrorHandlingAppliesToSubcommands(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "xf"}
	child := &cobra.Command{Use: "child"}
	grandchild := &cobra.Command{
		Use: "grandchild",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("runtime boom")
		},
	}

	child.AddCommand(grandchild)
	root.AddCommand(child)
	configureErrorHandling(root)

	if !grandchild.SilenceUsage {
		t.Error("expected SilenceUsage to be set on nested subcommands")
	}
}

func TestConfigureErrorHandlingIsIdempotent(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "exec <service> <command>",
		Args: cobra.MinimumNArgs(2),
	}

	configureErrorHandling(cmd)
	configureErrorHandling(cmd)

	err := cmd.Args(cmd, []string{"xf"})
	if err == nil {
		t.Fatal("expected an argument validation error")
	}

	count := 0
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, ok := current.(*usageError); ok {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("usage error was wrapped %d times, want exactly once", count)
	}
}
