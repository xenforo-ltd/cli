package build_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/build"
)

func TestBuildExecGeneratesArchiveContent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	root, addon := fixture(t, `{"exec":[["/bin/sh","-c","read -r input; printf '%s\\n' \"$PWD\" \"$input\" \"$1\" \"$2\" > generated.txt; printf stdout; printf stderr >&2","sh","{addon_id}:{version_string}:{version_id}:{enabled}","{missing}:{empty}:{object}"]]}`)
	put(t, addon, "addon.json", `{"version_string":"1.2 Beta","version_id":123,"enabled":true,"empty":"","object":{}}`)
	var stdout, stderr bytes.Buffer
	result, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall(), build.WithIO(strings.NewReader("input line\n"), &stdout, &stderr))
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "stdout" || stderr.String() != "stderr" {
		t.Fatalf("streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	archive, err := zip.OpenReader(result.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name != "upload/src/addons/Acme/Demo/generated.txt" {
			continue
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		canonicalAddon, err := filepath.EvalSymlinks(addon)
		if err != nil {
			t.Fatal(err)
		}
		want := canonicalAddon + "\ninput line\nAcme/Demo:1.2 Beta:123:true\n{missing}:{empty}:{object}\n"
		if string(data) != want {
			t.Fatalf("generated content: %q, want %q", data, want)
		}
		return
	}
	t.Fatal("generated file missing from archive")
}

func TestBuildExecStopsOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	root, addon := fixture(t, `{"exec":[["/bin/sh","-c","printf first > first.txt; exit 7"],["/bin/sh","-c","printf later > later.txt"]]}`)
	if _, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall()); err == nil {
		t.Fatal("expected command failure")
	}
	if _, err := os.Stat(filepath.Join(addon, "first.txt")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"later.txt", "hashes.json", "_releases"} {
		if _, err := os.Stat(filepath.Join(addon, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s exists or cannot be inspected: %v", name, err)
		}
	}
}

func TestBuildComposerOptional(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	bin := t.TempDir()
	composer := filepath.Join(bin, "composer")
	if err := os.WriteFile(composer, []byte("#!/bin/sh\nprintf 'composer:%s\\n' \"$*\"\nread -r input\nprintf 'input:%s\\n' \"$input\"\nprintf 'composer-error' >&2\nexit \"${COMPOSER_EXIT:-0}\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	for _, tc := range []struct {
		name                          string
		manifest, skip, fail, invoked bool
	}{
		{"no manifest", false, false, false, false},
		{"skipped", true, true, false, false},
		{"install", true, false, false, true},
		{"install fails", true, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, addon := fixture(t, `{}`)
			put(t, root, "src/XF.php", "<?php")
			if tc.manifest {
				put(t, addon, "composer.json", `{}`)
			}
			if tc.fail {
				t.Setenv("COMPOSER_EXIT", "9")
			} else {
				t.Setenv("COMPOSER_EXIT", "0")
			}
			var stdout, stderr bytes.Buffer
			opts := []build.Option{build.WithIO(strings.NewReader("hello\n"), &stdout, &stderr)}
			if tc.skip {
				opts = append(opts, build.WithoutDependencyInstall())
			}
			result, err := build.Build(context.Background(), root, "Acme/Demo", opts...)
			if (err != nil) != tc.fail {
				t.Fatalf("build error: %v", err)
			}
			if tc.invoked {
				lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
				if len(lines) != 2 || !strings.HasPrefix(lines[0], "composer:") || !slices.Contains(strings.Fields(strings.TrimPrefix(lines[0], "composer:")), "install") || !slices.Contains(strings.Fields(lines[0]), "--no-dev") || lines[1] != "input:hello" || stderr.String() != "composer-error" {
					t.Fatalf("streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			} else if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected composer output: %q %q", stdout.String(), stderr.String())
			}
			if tc.fail {
				if _, err := os.Stat(filepath.Join(addon, "_releases")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected release: %v", err)
				}
			} else if _, err := os.Stat(result.ZipPath); err != nil {
				t.Fatal(err)
			}
		})
	}
}
