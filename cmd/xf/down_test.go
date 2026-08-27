package main

import (
	"slices"
	"testing"
)

// TestDownComposeArgsLeaveVolumes guards the difference between stopping and
// destroying: xf down must keep volumes, or an environment meant to be started
// again would lose its database.
func TestDownComposeArgsLeaveVolumes(t *testing.T) {
	args := downComposeArgs()

	if !slices.Contains(args, "down") {
		t.Fatalf("down args %v do not invoke down", args)
	}

	if slices.Contains(args, "--volumes") {
		t.Errorf("down must not remove volumes: %v", args)
	}
}
