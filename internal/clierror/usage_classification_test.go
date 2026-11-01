package clierror

import (
	"errors"
	"testing"
)

// TestCancellationIsNotAUsageError guards the case that most clearly must not
// print usage: the user deliberately cancelled an interactive flow.
func TestCancellationIsNotAUsageError(t *testing.T) {
	t.Parallel()

	err := MarkAs(ErrCancelled, "initialization cancelled")

	if !errors.Is(err, ErrCancelled) {
		t.Fatal("expected the sentinel to be wrapped")
	}

	if IsUsage(err) {
		t.Error("cancelling an interactive flow must not print usage")
	}
}
