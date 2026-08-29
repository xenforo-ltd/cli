package dockercompose

import (
	"strings"
	"testing"
)

// TestDownArgsOmitVolumes documents that Down leaves volumes in place, which is
// correct for stopping an environment you intend to start again.
func TestDownArgsOmitVolumes(t *testing.T) {
	t.Parallel()

	args := downArgs(false)

	if !contains(args, "down") {
		t.Fatalf("args %v do not invoke down", args)
	}

	if contains(args, "--volumes") {
		t.Errorf("Down must not remove volumes: %v", args)
	}
}

// TestDestroyArgsRemoveVolumes covers permanent teardown. Without --volumes the
// database survives, so removing a worktree would leak one volume set per
// feature branch.
func TestDestroyArgsRemoveVolumes(t *testing.T) {
	t.Parallel()

	args := downArgs(true)

	if !contains(args, "--volumes") {
		t.Errorf("Destroy must remove volumes, got %v", args)
	}

	if !contains(args, "--remove-orphans") {
		t.Errorf("Destroy should remove orphaned containers, got %v", args)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if strings.TrimSpace(a) == want {
			return true
		}
	}

	return false
}
