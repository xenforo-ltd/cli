package build_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/build"
)

// TestBuildSymlinksWithinRoot checks contained file and directory links and a
// symlinked installation root.
func TestBuildSymlinksWithinRoot(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"whitelist":["addon.json","alias.txt"],"additional_files":["linked"]}`)
	writeFile(t, f.addon, "original.txt", "original")
	writeFile(t, f.addon, "directory/nested.txt", "nested")
	symlink(t, "original.txt", filepath.Join(f.addon, "alias.txt"))
	symlink(t, "directory", filepath.Join(f.addon, "linked"))
	result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
	if err != nil {
		t.Fatal(err)
	}
	entries := readArchive(t, result.ZipPath)
	for name, want := range map[string]string{"alias.txt": "original", "linked/nested.txt": "nested"} {
		entry := entries["upload/"+addOnPath+"/"+name]
		if entry.content != want || !entry.mode.IsRegular() {
			t.Errorf("%s = %+v, want regular file containing %q", name, entry, want)
		}
	}
	alias := filepath.Join(t.TempDir(), "installation")
	symlink(t, f.root, alias)
	second, err := build.Build(t.Context(), alias, addOnID, build.WithoutDependencyInstall())
	if err != nil {
		t.Fatal(err)
	}
	if *second != *result {
		t.Fatalf("symlinked installation changed result: %+v != %+v", second, result)
	}
}

// TestBuildRejectsSelectedSymlinkErrors checks broken, looping, and escaping
// links in selected sources and release extras.
func TestBuildRejectsSelectedSymlinkErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		config string
		link   string
		target string
		want   string
	}{
		{"broken selected file", `{"whitelist":["addon.json","bad"]}`, "bad", "missing", "reading bad"},
		{"selected loop", `{"whitelist":["**"]}`, "loop", ".", "symlink loop"},
		{"additional loop", `{"whitelist":["addon.json"],"additional_files":["loop"]}`, "loop", ".", "reading additional file"},
		{"broken additional file", `{"whitelist":["addon.json"],"additional_files":["bad"]}`, "bad", "missing", "reading additional file"},
		{"broken additional child", `{"whitelist":["addon.json"],"additional_files":["extra"]}`, "extra/bad", "missing", "reading additional file"},
		{"broken no upload file", `{"whitelist":["addon.json"]}`, "_no_upload/bad", "missing", "reading _no_upload"},
		{"escaping selected file", `{"whitelist":["addon.json","bad"]}`, "bad", "outside", "reading bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc.config)
			name := filepath.Join(f.addon, filepath.FromSlash(tc.link))
			if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				t.Fatal(err)
			}
			target := tc.target
			if target == "outside" {
				outside := t.TempDir()
				writeFile(t, outside, "outside.txt", "outside")
				target = filepath.Join(outside, "outside.txt")
			}
			symlink(t, target, name)
			_, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
			requireError(t, err, tc.want)
		})
	}
}

// TestBuildIgnoresUnselectedSymlinkErrors checks that unrelated or blacklisted
// link errors do not prevent a release.
func TestBuildIgnoresUnselectedSymlinkErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"whitelist":["addon.json","keep/**"],"blacklist":["keep/excluded"]}`)
	writeFile(t, f.addon, "keep/good.txt", "good")
	symlink(t, "missing", filepath.Join(f.addon, "unselected"))
	symlink(t, ".", filepath.Join(f.addon, "loop"))
	symlink(t, "missing", filepath.Join(f.addon, "keep", "excluded"))
	if _, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall()); err != nil {
		t.Fatal(err)
	}
}

// TestBuildJavaScriptSymlinkErrors checks that minify and rollup tasks report
// broken links in their inputs.
func TestBuildJavaScriptSymlinkErrors(t *testing.T) {
	t.Parallel()
	for _, config := range []string{
		`{"minify":["js/**/*.js"]}`,
		`{"rollup":{"bundle.js":["js/**/*.js"]}}`,
	} {
		t.Run(config, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, config)
			writeFile(t, f.addon, "js/good.js", "console.log(1);")
			symlink(t, "missing.js", filepath.Join(f.addon, "js", "bad.js"))
			requireError(t, build.BuildJavaScript(t.Context(), f.root, addOnID), "reading js/bad.js")
		})
	}
}
