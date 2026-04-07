package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveXenForoDirAndArgs_WithExplicitPath(t *testing.T) {
	root := t.TempDir()

	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(xfFile, []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	dir, args, err := resolveXenForoDirAndArgs([]string{root, "xf", "php", "-v"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if got, want := canonicalPath(t, dir), canonicalPath(t, root); got != want {
		t.Fatalf("dir = %q, want %q", got, want)
	}

	if !reflect.DeepEqual(args, []string{"xf", "php", "-v"}) {
		t.Fatalf("args = %v", args)
	}
}

func TestResolveXenForoDirAndArgs_AutoDetectsFromCWD(t *testing.T) {
	root := t.TempDir()

	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(xfFile, []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	t.Chdir(root)

	dir, args, err := resolveXenForoDirAndArgs([]string{"ps"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if got, want := canonicalPath(t, dir), canonicalPath(t, root); got != want {
		t.Fatalf("dir = %q, want %q", got, want)
	}

	if !reflect.DeepEqual(args, []string{"ps"}) {
		t.Fatalf("args = %v", args)
	}
}

func TestValidateExecInvocation(t *testing.T) {
	if err := validateExecInvocation([]string{"xf"}); err == nil {
		t.Fatal("expected error for missing command")
	} else if !errors.Is(err, ErrInvalidInput) {
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

		_, execArgs, err := resolveXenForoDirAndArgs([]string{"xf"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if err := validateExecInvocation(execArgs); err == nil {
			t.Fatal("expected validation failure")
		}
	})

	t.Run("exec path xf", func(t *testing.T) {
		_, execArgs, err := resolveXenForoDirAndArgs([]string{root, "xf"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if err := validateExecInvocation(execArgs); err == nil {
			t.Fatal("expected validation failure")
		}
	})

	t.Run("exec xf php -v", func(t *testing.T) {
		t.Chdir(root)

		_, execArgs, err := resolveXenForoDirAndArgs([]string{"xf", "php", "-v"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if err := validateExecInvocation(execArgs); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
}

func canonicalPath(t *testing.T, p string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}

	return filepath.Clean(resolved)
}
