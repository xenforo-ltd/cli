package build_test

import (
	"archive/zip"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/build"
)

func releaseContents(t *testing.T, path string) map[string]string {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	contents := make(map[string]string, len(archive.File))
	for _, entry := range archive.File {
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents[entry.Name] = string(data)
	}
	return contents
}

func TestBuildAdditionalGlobsAndFilePrecedence(t *testing.T) {
	root, addon := fixture(t, `{"whitelist":["js/two.js"],"additional_files":["js/*.js"]}`)
	put(t, addon, "js/one.js", "root one")
	put(t, addon, "js/two.js", "root two")
	put(t, addon, "js/ignored.txt", "not selected")
	put(t, addon, "_files/js/one.js", "replacement one")
	result, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall())
	if err != nil {
		t.Fatal(err)
	}
	contents := releaseContents(t, result.ZipPath)
	for name, want := range map[string]string{
		"upload/js/one.js":                      "replacement one",
		"upload/src/addons/Acme/Demo/js/two.js": "root two",
	} {
		if got, ok := contents[name]; !ok || got != want {
			t.Errorf("%s = %q (present %t), want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"upload/js/two.js", "upload/js/ignored.txt", "upload/src/addons/Acme/Demo/js/ignored.txt"} {
		if _, ok := contents[name]; ok {
			t.Errorf("unexpected archive entry %s", name)
		}
	}
}

func TestBuildHiddenFilesRequireExplicitWhitelist(t *testing.T) {
	t.Run("default whitelist excludes hidden paths", func(t *testing.T) {
		root, addon := fixture(t, `{}`)
		put(t, addon, "visible.txt", "visible")
		put(t, addon, ".secret", "hidden")
		put(t, addon, ".config/settings", "hidden directory")
		result, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall())
		if err != nil {
			t.Fatal(err)
		}
		contents := releaseContents(t, result.ZipPath)
		if _, ok := contents["upload/src/addons/Acme/Demo/visible.txt"]; !ok {
			t.Error("visible file missing")
		}
		for _, name := range []string{"upload/src/addons/Acme/Demo/.secret", "upload/src/addons/Acme/Demo/.config/settings"} {
			if _, ok := contents[name]; ok {
				t.Errorf("hidden file %s was included without being named", name)
			}
		}
	})
	t.Run("named hidden path is included", func(t *testing.T) {
		root, addon := fixture(t, `{"whitelist":[".htaccess","visible.txt"]}`)
		put(t, addon, ".htaccess", "rules")
		put(t, addon, "visible.txt", "visible")
		put(t, addon, ".secret", "hidden")
		result, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall())
		if err != nil {
			t.Fatal(err)
		}
		contents := releaseContents(t, result.ZipPath)
		if got := contents["upload/src/addons/Acme/Demo/.htaccess"]; got != "rules" {
			t.Errorf(".htaccess = %q, want its content", got)
		}
		if _, ok := contents["upload/src/addons/Acme/Demo/.secret"]; ok {
			t.Error("unnamed hidden file was included")
		}
	})
}

func TestBuildAdditionalGlobMustMatch(t *testing.T) {
	root, _ := fixture(t, `{"additional_files":["assets/*.js"]}`)
	_, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall())
	if err == nil || !strings.Contains(err.Error(), "matched no files") {
		t.Fatalf("unmatched glob error = %v", err)
	}
}
