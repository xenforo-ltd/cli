package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clierrors "xf/internal/errors"
)

func TestFindXenForoDirFindsParent(t *testing.T) {
	t.Setenv("XF_DIR", "")

	root := t.TempDir()
	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(xfFile, []byte("<?php"), 0644); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	detected, err := findXenForoDir(nested)
	if err != nil {
		t.Fatalf("findXenForoDir returned error: %v", err)
	}
	if detected != root {
		t.Fatalf("detected dir = %q, want %q", detected, root)
	}
}

func TestFindXenForoDirTerminatesAtRoot(t *testing.T) {
	t.Setenv("XF_DIR", "")

	done := make(chan struct{})
	go func() {
		_, _ = findXenForoDir(string(filepath.Separator))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("findXenForoDir did not terminate")
	}
}

func TestRunAsXenForoCommandOutsideDirReturnsActionableError(t *testing.T) {
	t.Setenv("XF_DIR", "")
	setCwd(t, t.TempDir())

	err := runAsXenForoCommand([]string{"list"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !clierrors.Is(err, clierrors.CodeInvalidInput) {
		t.Fatalf("expected invalid input code, got: %v", err)
	}
	if got := err.Error(); got == "" || !containsAll(got, "unknown command", "not in a XenForo directory") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestIsKnownCommandIncludesCobraCompletion(t *testing.T) {
	if !isKnownCommand("completion") {
		t.Fatal("expected completion to be treated as a known command")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
