package xf

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetXenForoDirFindsParent(t *testing.T) {
	t.Setenv("XF_DIR", "")

	root := t.TempDir()
	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(xfFile, []byte("<?php"), 0644); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	nested := filepath.Join(root, "nested", "path")
	if err := os.MkdirAll(nested, 0755); err != nil {
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
		_, _ = GetXenForoDir(string(filepath.Separator))
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
