package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
)

// exitErrorWithCode produces a real *exec.ExitError carrying the given status,
// so the test exercises the same type the docker runner surfaces.
func exitErrorWithCode(t *testing.T, code int) error {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "sh", "-c", fmt.Sprintf("exit %d", code))

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected a non-zero exit for code %d", code)
	}

	return err
}

func TestPassthroughErrorPreservesTheChildExitCode(t *testing.T) {
	for _, code := range []int{1, 2, 3, 42} {
		err := passthroughError(exitErrorWithCode(t, code), "failed to run PHP")

		var exitErr *exitCodeError
		if !errors.As(err, &exitErr) {
			t.Fatalf("code %d: got %T, want *exitCodeError", code, err)
		}

		if exitErr.code != code {
			t.Errorf("exit code = %d, want %d", exitErr.code, code)
		}
	}
}

func TestPassthroughErrorPreservesTheChildExitCodeThroughWrapping(t *testing.T) {
	// The docker runner wraps the child's error before it reaches us.
	wrapped := fmt.Errorf("docker command failed: %w", exitErrorWithCode(t, 7))

	var exitErr *exitCodeError
	if !errors.As(passthroughError(wrapped, "failed to run Composer"), &exitErr) {
		t.Fatal("wrapped exit error was not recognised")
	}

	if exitErr.code != 7 {
		t.Errorf("exit code = %d, want 7", exitErr.code)
	}
}

func TestPassthroughErrorKeepsNonExitFailuresReportable(t *testing.T) {
	// A failure that is not a child exit - the daemon being unreachable, or
	// the command being cancelled - must stay visible rather than becoming a
	// silent exit code.
	err := passthroughError(context.Canceled, "failed to run docker compose")

	var exitErr *exitCodeError
	if errors.As(err, &exitErr) {
		t.Fatal("cancellation was converted to a silent exit code")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("underlying error lost: %v", err)
	}

	if got := err.Error(); got != "failed to run docker compose: context canceled" {
		t.Errorf("message = %q", got)
	}
}

func TestPassthroughErrorPassesNilThrough(t *testing.T) {
	if err := passthroughError(nil, "failed to run PHP"); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}
