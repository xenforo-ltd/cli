package passthrough

import (
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

	dir, args, err := Resolve([]string{root, "xf", "php", "-v"})
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

	dir, args, err := Resolve([]string{"ps"})
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
			if got := stripSeparator(tt.args); !reflect.DeepEqual(got, tt.want) {
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

		_, args, err := Resolve([]string{"--", "install"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !reflect.DeepEqual(args, []string{"install"}) {
			t.Fatalf("args = %v", args)
		}
	})

	t.Run("separator after path", func(t *testing.T) {
		_, args, err := Resolve([]string{root, "--", "install"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if !reflect.DeepEqual(args, []string{"install"}) {
			t.Fatalf("args = %v", args)
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

func TestFlagSeparatorStillAccepted(t *testing.T) {
	got := []string{"--", "-v"}
	if stripped := stripSeparator(got); !reflect.DeepEqual(stripped, []string{"-v"}) {
		t.Errorf("stripFlagSeparator(%v) = %v, want [-v]", got, stripped)
	}
}
