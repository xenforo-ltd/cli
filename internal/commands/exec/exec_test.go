package exec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/passthrough"
)

func TestValidateExecInvocation(t *testing.T) {
	if err := validateExecInvocation([]string{"xf"}); err == nil {
		t.Fatal("expected error for missing command")
	} else if !errors.Is(err, clierror.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if err := validateExecInvocation([]string{"xf", "php", "-v"}); err != nil {
		t.Fatalf("unexpected error for valid invocation: %v", err)
	}
}

func TestExecInvocationScenarios(t *testing.T) {
	root := t.TempDir()
	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(xfFile, []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}
	t.Run("exec xf", func(t *testing.T) {
		t.Chdir(root)
		_, execArgs, err := passthrough.Resolve([]string{"xf"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if err := validateExecInvocation(execArgs); err == nil {
			t.Fatal("expected validation failure")
		}
	})
	t.Run("exec path xf", func(t *testing.T) {
		_, execArgs, err := passthrough.Resolve([]string{root, "xf"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if err := validateExecInvocation(execArgs); err == nil {
			t.Fatal("expected validation failure")
		}
	})
	t.Run("exec xf php -v", func(t *testing.T) {
		t.Chdir(root)
		_, execArgs, err := passthrough.Resolve([]string{"xf", "php", "-v"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if err := validateExecInvocation(execArgs); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
}
