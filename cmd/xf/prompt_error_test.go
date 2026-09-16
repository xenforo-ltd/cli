package main

import (
	"errors"
	"testing"

	"charm.land/huh/v2"
)

// TestPromptErrorTreatsCtrlCAsInterrupt covers Ctrl-C at an interactive
// prompt. huh runs the terminal in raw mode, so no SIGINT is delivered and the
// abort arrives as huh.ErrUserAborted; it must still exit like any other
// interrupt (130), not as a quiet exit 0 cancellation.
func TestPromptErrorTreatsCtrlCAsInterrupt(t *testing.T) {
	t.Parallel()

	err := promptError(huh.ErrUserAborted, "license selection")

	if !isInterrupted(err) {
		t.Fatalf("Ctrl-C at a prompt must be an interrupt, got %v", err)
	}

	if errors.Is(err, ErrCancelled) {
		t.Fatalf("Ctrl-C at a prompt must not be a quiet cancellation, got %v", err)
	}

	if msg := err.Error(); msg != "license selection interrupted" {
		t.Fatalf("unexpected message %q", msg)
	}
}

// TestPromptErrorReportsOtherFailures covers a prompt that could not run at
// all, e.g. `xf init` under CI with no /dev/tty. That is a failure to report
// with its cause, not a cancellation that exits 0 having done nothing.
func TestPromptErrorReportsOtherFailures(t *testing.T) {
	t.Parallel()

	cause := errors.New("huh: open /dev/tty: no such device or address")
	err := promptError(cause, "version selection for %s", "xfmg")

	if isInterrupted(err) || errors.Is(err, ErrCancelled) {
		t.Fatalf("a non-abort prompt failure must be reported, got %v", err)
	}

	if !errors.Is(err, cause) {
		t.Fatalf("cause must be preserved, got %v", err)
	}

	if msg := err.Error(); msg != "version selection for xfmg: "+cause.Error() {
		t.Fatalf("unexpected message %q", msg)
	}
}

func TestPromptErrorNilIsNotProduced(t *testing.T) {
	t.Parallel()

	// Callers only reach promptError with a non-nil error, but a nil input
	// must not be classified as an interrupt or cancellation either.
	err := promptError(nil, "review")
	if isInterrupted(err) || errors.Is(err, ErrCancelled) {
		t.Fatalf("nil must not classify as interrupt or cancellation, got %v", err)
	}
}
