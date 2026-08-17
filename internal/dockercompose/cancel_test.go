package dockercompose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newRunnerForCancel builds a Runner against a throwaway XenForo directory and
// puts a fake `docker` on PATH that ignores termination until killed, mimicking
// a long-running `docker compose logs -f`.
func newRunnerForCancel(t *testing.T) *Runner {
	t.Helper()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n"), 0o600); err != nil {
		t.Fatalf("write compose.yaml: %v", err)
	}

	binDir := t.TempDir()

	script := "#!/bin/sh\nwhile true; do sleep 0.05; done\n"
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}

	t.Setenv("PATH", binDir)

	runner, err := NewRunner(dir)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	return runner
}

// TestCancelledCommandReportsContextError covers interrupting a long-running
// command with Ctrl-C.
//
// exec.CommandContext kills the child when the context is cancelled, which
// surfaces as "signal: killed". That says nothing useful and is not a failure of
// the command, so the context's own error must be reported instead. Callers can
// then recognise the cancellation and exit quietly.
func TestCancelledCommandReportsContextError(t *testing.T) {
	runner := newRunnerForCancel(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runner.Logs(ctx, true)
	if err == nil {
		t.Fatal("expected an error when the context is cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %q does not unwrap to context.Canceled", err)
	}
}

// TestDeadlineExceededIsReported covers the same path for a timeout, so a slow
// command is distinguishable from a user interrupt.
func TestDeadlineExceededIsReported(t *testing.T) {
	runner := newRunnerForCancel(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runner.Logs(ctx, true)
	if err == nil {
		t.Fatal("expected an error when the deadline is exceeded")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %q does not unwrap to context.DeadlineExceeded", err)
	}
}
