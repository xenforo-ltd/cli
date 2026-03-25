package xf

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/testutils"
)

func TestGetXenForoDirFindsParent(t *testing.T) {
	root := testutils.SetupXenForoDir(t)

	nested := filepath.Join(root, "nested", "path")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	detected, err := GetXenForoDir(nested)
	if err != nil {
		t.Fatalf("GetXenForoDir returned error: %v", err)
	}

	if detected != root {
		t.Fatalf("detected dir = %q, want %q", detected, root)
	}
}

func TestGetXenForoDirTerminatesAtRoot(t *testing.T) {
	t.Setenv("XF_DIR", "")

	done := make(chan struct{})

	go func() {
		if _, err := GetXenForoDir(string(filepath.Separator)); err != nil {
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

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "plain", want: false},
		{value: "has space", want: true},
		{value: "$VAR", want: true},
		{value: "${VAR}", want: false},
	}

	for _, tt := range tests {
		if got := needsQuoting(tt.value); got != tt.want {
			t.Fatalf("needsQuoting(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
