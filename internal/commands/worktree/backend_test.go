package worktree

import "testing"

// backendFor selects the backend for dir, failing the test if detection fails.
// Tests exercise the backend seam directly, so they do not need the exported
// wrappers the production code dropped.
func backendFor(t *testing.T, dir string) backend {
	t.Helper()

	b, err := detectBackend(t.Context(), dir)
	if err != nil {
		t.Fatalf("detectBackend(%q): %v", dir, err)
	}

	return b
}
