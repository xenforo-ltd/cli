package main

import (
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
)

func TestWorktreeCreateJSONRequiresNoSetup(t *testing.T) {
	originalJSON := flagWorktreeJSON
	originalNoSetup := flagWorktreeNoSetup
	t.Cleanup(func() {
		flagWorktreeJSON = originalJSON
		flagWorktreeNoSetup = originalNoSetup
	})
	flagWorktreeJSON = true
	flagWorktreeNoSetup = false
	err := runWorktreeCreate(worktreeCreateCmd, nil)
	if err == nil {
		t.Fatal("expected --json without --no-setup to be rejected")
	}
	if !clierror.IsUsage(err) {
		t.Errorf("expected a usage error so usage is printed, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "--no-setup") {
		t.Errorf("error %q does not name the flag --json requires", err)
	}
}
