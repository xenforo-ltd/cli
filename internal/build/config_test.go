package build_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/build"
)

// TestBuildInvalidConfiguration checks that malformed and unsafe build settings
// and metadata are rejected.
func TestBuildInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		config   string
		metadata string
		want     string
	}{
		{"invalid JSON", `{`, "", "parsing build.json"},
		{"null build settings", `null`, "", "expected an object"},
		{"wrong field type", `{"whitelist":true}`, "", "parsing build.json"},
		{"empty root", `{"root":""}`, "", "must be relative"},
		{"absolute root", `{"root":"/outside"}`, "", "must be relative"},
		{"backslash root", `{"root":"..\\.."}`, "", "must be relative"},
		{"root outside installation", `{"root":"../../../../.."}`, "", "must stay within"},
		{"root below addon", `{"root":"child"}`, "", "must contain the add-on"},
		{"empty whitelist pattern", `{"whitelist":[""]}`, "", "invalid whitelist"},
		{"parent whitelist pattern", `{"whitelist":["../*"]}`, "", "parent directory references"},
		{"absolute whitelist pattern", `{"whitelist":["/foo"]}`, "", "must be relative"},
		{"negated whitelist pattern", `{"whitelist":["!foo"]}`, "", "unsupported negation"},
		{"malformed whitelist pattern", `{"whitelist":["["]}`, "", "invalid whitelist"},
		{"malformed blacklist", `{"blacklist":["["]}`, "", "invalid blacklist"},
		{"hash exclusions must be an array", `{"hash_exclude":"file.txt"}`, "", "parsing build.json"},
		{"malformed hash exclusion", `{"hash_exclude":["["]}`, "", "invalid hash_exclude"},
		{"hash exclusion traversal", `{"hash_exclude":["../file.txt"]}`, "", "invalid hash_exclude"},
		{"invalid minify string", `{"minify":"other"}`, "", "invalid minify"},
		{"invalid minify type", `{"minify":true}`, "", "invalid minify"},
		{"invalid minify pattern", `{"minify":["["]}`, "", "invalid minify"},
		{"rollup array", `{"rollup":[]}`, "", "rollup must be an object"},
		{"rollup input type", `{"rollup":{"out.js":"input.js"}}`, "", "rollup output"},
		{"rollup output traversal", `{"rollup":{"../out.js":["input.js"]}}`, "", "invalid rollup output"},
		{"rollup output glob", `{"rollup":{"*.js":["input.js"]}}`, "", "ending in .js"},
		{"rollup output extension", `{"rollup":{"out.txt":["input.js"]}}`, "", "ending in .js"},
		{"duplicate rollup output", `{"rollup":{"out.js":["a.js"],"./out.js":["b.js"]}}`, "", "duplicates"},
		{"invalid rollup input", `{"rollup":{"out.js":["["]}}`, "", "rollup output"},
		{"empty rollup inputs", `{"rollup":{"out.js":[]}}`, "", "has no inputs"},
		{"additional traversal", `{"additional_files":["../file"]}`, "", "invalid additional file"},
		{"additional backslash", `{"additional_files":["foo\\bar"]}`, "", "invalid additional file"},
		{"additional NUL", `{"additional_files":["foo\u0000bar"]}`, "", "invalid additional file"},
		{"additional drive", `{"additional_files":["C:foo"]}`, "", "invalid additional file"},
		{"invalid metadata", `{}`, `{`, "parsing addon.json"},
		{"trailing metadata", `{}`, `{"version_string":"1.0"} {}`, "unexpected data after metadata"},
		{"null metadata", `{}`, `null`, "expected an object"},
		{"missing version", `{}`, `{}`, "usable release version"},
		{"invalid version type", `{}`, `{"version_string":1}`, "usable release version"},
		{"unusable version", `{}`, `{"version_string":"/!?"}`, "usable release version"},
		{"invalid autoload type", `{}`, `{"title":"Demo","version_id":100,"version_string":"1.0","composer_autoload":false}`, "composer_autoload must be a string"},
		{"autoload traversal", `{}`, `{"title":"Demo","version_id":100,"version_string":"1.0","composer_autoload":"../vendor/composer"}`, "invalid addon.json composer_autoload"},
		{"absolute autoload", `{}`, `{"title":"Demo","version_id":100,"version_string":"1.0","composer_autoload":"/vendor/composer"}`, "invalid addon.json composer_autoload"},
		{"empty command", `{"exec":[[]]}`, "", "commands must not be empty"},
		{"blank command", `{"exec":[["   "]]}`, "", "commands must not be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc.config)
			if tc.metadata != "" {
				writeFile(t, f.addon, "addon.json", tc.metadata)
			}
			requireError(t, build.BuildJavaScript(t.Context(), f.root, addOnID), tc.want)
		})
	}
}

// TestPublicAPIsInvalidInstallation checks add-on IDs and installation paths
// through each entry point.
func TestPublicAPIsInvalidInstallation(t *testing.T) {
	t.Parallel()
	apis := []struct {
		name string
		run  func(context.Context, string, string) error
	}{
		{"Build", func(ctx context.Context, root, id string) error {
			_, err := build.Build(ctx, root, id, build.WithoutDependencyInstall())
			return err
		}},
		{"BuildJavaScript", build.BuildJavaScript},
		{"WatchJavaScript", func(ctx context.Context, root, id string) error {
			return build.WatchJavaScript(ctx, root, id, build.WatchOptions{})
		}},
	}
	for _, api := range apis {
		t.Run(api.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, `{}`)
			requireError(t, api.run(t.Context(), f.root, "../Bad"), "invalid add-on ID")
			requireError(t, api.run(t.Context(), filepath.Join(f.root, "missing"), addOnID), "opening XenForo root")
			requireError(t, api.run(t.Context(), filepath.Join(f.addon, "addon.json"), addOnID), "opening XenForo root")
		})
	}
}

// TestBuildMissingConfiguration checks that missing build.json uses defaults,
// while missing metadata and unreadable configuration still fail.
func TestBuildMissingConfiguration(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"build.json", "addon.json"} {
		for _, kind := range []string{"missing", "directory", "broken symlink"} {
			t.Run(name+" "+kind, func(t *testing.T) {
				t.Parallel()
				f := newFixture(t, `{}`)
				filename := filepath.Join(f.addon, name)
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					if err := os.Mkdir(filename, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "broken symlink" {
					symlink(t, "missing.json", filename)
				}
				if name == "build.json" && kind == "missing" {
					writeFile(t, f.addon, "Demo.php", "<?php")
					writeFile(t, f.addon, "js/demo.js", "console.log(1 + 2);")
					writeFile(t, f.addon, "_output/.metadata.json", "excluded")
					writeFile(t, f.addon, ".htaccess", "excluded by default")
					writeFile(t, f.addon, ".git/objects/data", "excluded by default")
					if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
						t.Fatal(err)
					}
					requireMissing(t, f.addon, "js/demo.min.js")
					result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
					if err != nil {
						t.Fatal(err)
					}
					entries := readArchive(t, result.ZipPath)
					dataFiles, err := os.ReadDir(filepath.Join(f.addon, "_data"))
					if err != nil {
						t.Fatal(err)
					}
					if len(entries) != 4+len(dataFiles) || entries["upload/"+addOnPath+"/Demo.php"].content != "<?php" {
						t.Fatalf("unexpected default release contents: %#v", entries)
					}
					return
				}
				requireError(t, build.BuildJavaScript(t.Context(), f.root, addOnID), "reading "+name)
			})
		}
	}
}
