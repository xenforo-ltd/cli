package main

import (
	"errors"
	"os/exec"
	"testing"
)

func TestPassthroughErrorIgnoresSignalExitCodes(t *testing.T) {
	// A process killed by a signal reports -1, which cannot be used as a
	// process exit status. Those failures must stay reportable instead.
	cmd := exec.Command("sh", "-c", "kill -TERM $$")

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected the signalled command to fail")
	}

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected an *exec.ExitError, got %T", err)
	}

	if ee.ExitCode() != -1 {
		t.Skipf("platform reported exit code %d, not -1", ee.ExitCode())
	}

	if exitErr, ok := errors.AsType[*exitCodeError](passthroughError(err, "failed to run PHP")); ok {
		t.Errorf("signal death became exit code %d", exitErr.code)
	}
}
