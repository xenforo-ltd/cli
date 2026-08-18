package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/testutils"
	"github.com/xenforo-ltd/cli/internal/xf"
)

func TestCommandsAreGrouped(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("command %q has no GroupID", c.Name())
		}
	}
}

func TestFirstErrorClause(t *testing.T) {
	in := "failed to start Docker environment: docker command failed: exit status 1"
	if got := firstErrorClause(in); got != "failed to start Docker environment" {
		t.Errorf("got %q", got)
	}
	if got := firstErrorClause("plain message"); got != "plain message" {
		t.Errorf("got %q", got)
	}
}

func TestFindXenForoDirFindsParent(t *testing.T) {
	root := testutils.SetupXenForoDir(t)

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	detected, err := xf.GetXenForoDir(nested)
	if err != nil {
		t.Fatalf("GetXenForoDir returned error: %v", err)
	}

	if detected != root {
		t.Fatalf("detected dir = %q, want %q", detected, root)
	}
}

func TestFindXenForoDirTerminatesAtRoot(t *testing.T) {
	t.Setenv("XF_DIR", "")

	done := make(chan struct{})

	go func() {
		if _, err := xf.GetXenForoDir(string(filepath.Separator)); err != nil {
			// Error is expected; we're testing that the function terminates.
			t.Logf("GetXenForoDir: %v", err)
		}

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetXenForoDir did not terminate")
	}
}

func TestRunAsXenForoCommandOutsideDirReturnsActionableError(t *testing.T) {
	t.Setenv("XF_DIR", "")
	t.Chdir(t.TempDir())

	err := runAsXenForoCommand(t.Context(), []string{"list"}, exec.CommandContext)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, xf.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got: %v", err)
	}

	if got := err.Error(); got == "" || !containsAll(got, "unknown command", "not in a XenForo directory") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestIsKnownCommandIncludesCobraCompletion(t *testing.T) {
	if !isKnownCommand("completion") {
		t.Fatal("expected completion to be treated as a known command")
	}
}

func TestRunAsXenForoCommandFallsBackToLocalWhenComposeMissing(t *testing.T) {
	root := testutils.SetupXenForoDir(t)

	t.Chdir(root)

	cmdFn := helperCommand(t,
		"php cmd.php xf-dev:import",
		root,
		0,
	)

	if err := runAsXenForoCommand(t.Context(), []string{"xf-dev:import"}, cmdFn); err != nil {
		t.Fatalf("runAsXenForoCommand returned error: %v", err)
	}
}

func TestRunAsLocalXenForoCommandBuildsExpectedInvocation(t *testing.T) {
	root := t.TempDir()
	cmdFn := helperCommand(t,
		"php cmd.php cron:run --verbose",
		root,
		0,
	)

	if err := runAsLocalXenForoCommand(t.Context(), root, []string{"cron:run", "--verbose"}, cmdFn); err != nil {
		t.Fatalf("runAsLocalXenForoCommand returned error: %v", err)
	}
}

func TestRunAsLocalXenForoCommandReturnsActionableErrorWhenPHPMissing(t *testing.T) {
	root := t.TempDir()
	cmdFn := func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "__xf_missing_php_binary__")
	}

	err := runAsLocalXenForoCommand(t.Context(), root, []string{"list"}, cmdFn)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("expected invalid input error, got: %v", err)
	}

	if got := err.Error(); !containsAll(got, "PHP", "PATH") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestRunAsLocalXenForoCommandReturnsErrorOnNonZeroExit(t *testing.T) {
	root := t.TempDir()
	cmdFn := helperCommand(t,
		"php cmd.php list",
		root,
		2,
	)

	err := runAsLocalXenForoCommand(t.Context(), root, []string{"list"}, cmdFn)
	if err == nil {
		t.Fatal("expected error")
	}

	if !containsAll(err.Error(), "local XenForo command failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}

	return true
}

func helperCommand(t *testing.T, expectedArgs, expectedWd string, exitCode int) commandFunc {
	t.Helper()
	expectedWd = canonicalPath(t, expectedWd)

	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cs := make([]string, 0, len(args)+3)
		cs = append(cs, "-test.run=TestHelperProcess", "--", command)
		cs = append(cs, args...)

		cmd := exec.CommandContext(ctx, os.Args[0], cs...)

		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_EXPECT_ARGS="+expectedArgs,
			"HELPER_EXPECT_WD="+expectedWd,
			fmt.Sprintf("HELPER_EXIT_CODE=%d", exitCode),
		)

		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	dash := -1

	for i, arg := range os.Args {
		if arg == "--" {
			dash = i
			break
		}
	}

	if dash == -1 {
		fmt.Fprintln(os.Stderr, "missing -- separator")
		os.Exit(2)
	}

	gotArgs := strings.Join(os.Args[dash+1:], " ")
	if wantArgs := os.Getenv("HELPER_EXPECT_ARGS"); wantArgs != "" && gotArgs != wantArgs {
		fmt.Fprintf(os.Stderr, "args mismatch: got %q want %q\n", gotArgs, wantArgs)
		os.Exit(3)
	}

	if wantWd := os.Getenv("HELPER_EXPECT_WD"); wantWd != "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "getwd failed: %v\n", err)
			os.Exit(4)
		}

		if resolved, err := filepath.EvalSymlinks(wd); err == nil {
			wd = filepath.Clean(resolved)
		} else {
			wd = filepath.Clean(wd)
		}

		if wd != filepath.Clean(wantWd) {
			fmt.Fprintf(os.Stderr, "cwd mismatch: got %q want %q\n", wd, filepath.Clean(wantWd))
			os.Exit(5)
		}
	}

	code := 0
	if _, err := fmt.Sscanf(os.Getenv("HELPER_EXIT_CODE"), "%d", &code); err != nil {
		fmt.Fprintf(os.Stderr, "invalid HELPER_EXIT_CODE: %v\n", err)
		os.Exit(6)
	}

	os.Exit(code)
}

func TestInterruptExitCodeFollowsTheSignal(t *testing.T) {
	t.Cleanup(func() { interruptSignal = atomic.Value{} })

	if got := interruptExitCode(); got != exitInterrupted {
		t.Errorf("with no signal recorded, got %d, want %d", got, exitInterrupted)
	}

	recordInterruptSignal(syscall.SIGINT)

	if got := interruptExitCode(); got != 130 {
		t.Errorf("after SIGINT, got %d, want 130", got)
	}

	recordInterruptSignal(syscall.SIGTERM)

	if got := interruptExitCode(); got != 143 {
		t.Errorf("after SIGTERM, got %d, want 143", got)
	}
}
