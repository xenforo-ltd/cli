package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
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

func TestStripFlagSeparator(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "empty", args: nil, want: nil},
		{name: "no separator", args: []string{"install"}, want: []string{"install"}},
		{name: "leading separator", args: []string{"--", "install"}, want: []string{"install"}},
		{name: "separator only", args: []string{"--"}, want: []string{}},
		{name: "flag untouched", args: []string{"--direct"}, want: []string{"--direct"}},
		{name: "separator after args untouched", args: []string{"install", "--"}, want: []string{"install", "--"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripFlagSeparator(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("stripFlagSeparator(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestResolveXenForoDirAndArgs_StripsSeparator(t *testing.T) {
	root := t.TempDir()

	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(xfFile, []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	t.Run("leading separator without path", func(t *testing.T) {
		t.Chdir(root)

		_, args, err := resolveXenForoDirAndArgs([]string{"--", "install"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !reflect.DeepEqual(args, []string{"install"}) {
			t.Fatalf("args = %v", args)
		}
	})

	t.Run("separator after path", func(t *testing.T) {
		_, args, err := resolveXenForoDirAndArgs([]string{root, "--", "install"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !reflect.DeepEqual(args, []string{"install"}) {
			t.Fatalf("args = %v", args)
		}
	})
}

func TestPassthroughCommandsForwardFlags(t *testing.T) {
	commands := []*cobra.Command{
		composerCmd,
		phpCmd,
		phpDebugCmd,
		composeCmd,
		execCmd,
		debugCmd,
	}

	for _, cmd := range commands {
		t.Run(cmd.Name(), func(t *testing.T) {
			if err := cmd.ParseFlags([]string{"outdated", "--direct"}); err != nil {
				t.Fatalf("ParseFlags failed: %v", err)
			}

			if got := cmd.Flags().Args(); !reflect.DeepEqual(got, []string{"outdated", "--direct"}) {
				t.Fatalf("args = %v, want %v", got, []string{"outdated", "--direct"})
			}
		})
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
