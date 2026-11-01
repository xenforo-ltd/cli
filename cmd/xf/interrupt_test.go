package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
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
