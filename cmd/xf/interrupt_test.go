package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// TestInterruptIsNotReportedAsAnError covers Ctrl-C during a long-running
// command such as `xf logs --follow`.
//
// Cancelling is a deliberate user action, so it must not print an error or be
// treated as a failure, however deeply the context error is wrapped.
func TestInterruptIsNotReportedAsAnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "bare cancellation", err: context.Canceled, want: true},
		{
			name: "wrapped cancellation",
			err:  fmt.Errorf("failed to show container logs: %w", context.Canceled),
			want: true,
		},
		{
			name: "doubly wrapped cancellation",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", context.Canceled)),
			want: true,
		},
		{
			name: "deadline exceeded is a real failure",
			err:  context.DeadlineExceeded,
			want: false,
		},
		{
			name: "ordinary failure",
			err:  errors.New("docker command failed: exit status 1"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isInterrupted(tt.err); got != tt.want {
				t.Errorf("isInterrupted(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestLocalCommandReportsCancellation covers the non-Dockerised fallback, which
// runs `php cmd.php ...` directly. Cancellation must reach that child too, and
// be reported as a cancellation rather than as a failed command.
func TestLocalCommandReportsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	cmdFn := func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "while true; do sleep 0.05; done")
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runAsLocalXenForoCommand(ctx, t.TempDir(), []string{"list"}, cmdFn)
	if err == nil {
		t.Fatal("expected an error when the context is cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %q does not unwrap to context.Canceled", err)
	}

	if !isInterrupted(err) {
		t.Error("a cancelled local command must be treated as an interrupt")
	}
}
