package build_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/build"
)

// TestBuildRelease checks archive selection, contents, permissions, hashes, and
// result statistics, including a repeated build.
func TestBuildRelease(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{
		"whitelist":["./**","**/.htaccess",".allowed/**",".allowed/.inner/**",".partial/selected.txt"],
		"blacklist":["excluded/","omit.txt",".allowed/omit.txt"],
		"hash_exclude":["**/.htaccess","extra.txt","js/demo/**"],
		"additional_files":["js/demo","extra.txt",".ignored"],
		"minify":["scripts/*.js"]
	}`)
	writeFile(t, f.addon, "addon.json", `{"version_string":" 1.0 / beta! "}`)
	for name, content := range map[string]string{
		"Demo.php":                  "<?php\r\n// Demo\r\n",
		"scripts/demo.js":           "function demo() { return 1 + 2; }\n",
		"bin/run":                   "run\n",
		".htaccess":                 "deny\n",
		".secret":                   "secret",
		".hidden/file.txt":          "hidden",
		".git/objects/data":         "repository metadata",
		".allowed/keep.txt":         "explicit hidden directory",
		".allowed/omit.txt":         "blacklisted hidden directory file",
		".allowed/.secret":          "not explicitly selected",
		".allowed/.inner/keep.txt":  "explicit nested hidden directory",
		".partial/selected.txt":     "explicit hidden file selection",
		".partial/unselected.txt":   "not selected by a broad wildcard",
		"excluded/nested/file.txt":  "excluded",
		"omit.txt":                  "omitted",
		"_output/.ignored.txt":      "generated metadata",
		"_stubs/file.txt":           "stub",
		"hashes.json":               "old manifest",
		"_files/js/demo/source.js":  "console.log('extra');\n",
		"_files/js/demo/.hidden":    "hidden",
		"_files/js/demo/.htaccess":  "allow\n",
		"_files/js/demo/build.json": "excluded",
		"_files/extra.txt":          "preferred",
		"extra.txt":                 "fallback",
		"_no_upload/README.txt":     "read me\n",
		"_no_upload/.hidden":        "hidden",
		"_no_upload/.htaccess":      "explicit archive extra",
	} {
		writeFile(t, f.addon, name, content)
	}
	if err := os.Chmod(filepath.Join(f.addon, "bin", "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
	if err != nil {
		t.Fatal(err)
	}
	realAddon, err := filepath.EvalSymlinks(f.addon)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(realAddon, "hashes.json"); result.HashesPath != want {
		t.Fatalf("HashesPath = %q, want %q", result.HashesPath, want)
	}
	if want := filepath.Join(realAddon, "_releases", "Vendor-Demo-1.0 beta.zip"); result.ZipPath != want {
		t.Fatalf("ZipPath = %q, want %q", result.ZipPath, want)
	}
	entries := readArchive(t, result.ZipPath)
	for name, entry := range entries {
		if !entry.modified.Equal(time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("%s has a non-default ZIP timestamp: %s", name, entry.modified)
		}
	}
	upload := "upload/" + addOnPath + "/"
	want := map[string]string{
		upload + "addon.json":               readFile(t, f.addon, "addon.json"),
		upload + "Demo.php":                 readFile(t, f.addon, "Demo.php"),
		upload + "scripts/demo.js":          readFile(t, f.addon, "scripts/demo.js"),
		upload + "scripts/demo.min.js":      readFile(t, f.addon, "scripts/demo.min.js"),
		upload + "scripts/demo.min.js.map":  readFile(t, f.addon, "scripts/demo.min.js.map"),
		upload + "bin/run":                  "run\n",
		upload + ".htaccess":                "deny\n",
		upload + ".allowed/keep.txt":        "explicit hidden directory",
		upload + ".allowed/.inner/keep.txt": "explicit nested hidden directory",
		upload + ".partial/selected.txt":    "explicit hidden file selection",
		upload + "extra.txt":                "fallback",
		"upload/js/demo/source.js":          "console.log('extra');\n",
		"upload/js/demo/.htaccess":          "allow\n",
		"upload/extra.txt":                  "preferred",
		"README.txt":                        "read me\n",
		".htaccess":                         "explicit archive extra",
	}
	dataFiles, err := os.ReadDir(filepath.Join(f.addon, "_data"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range dataFiles {
		name := "_data/" + file.Name()
		want[upload+name] = readFile(t, f.addon, name)
	}
	manifestName := upload + "hashes.json"
	manifest, exists := entries[manifestName]
	if !exists {
		t.Fatal("ZIP is missing hashes.json")
	}
	if manifest.content != readFile(t, f.addon, "hashes.json") {
		t.Fatal("ZIP manifest differs from the generated manifest")
	}
	delete(entries, manifestName)
	contents := make(map[string]string)
	var total int64
	for name, entry := range entries {
		contents[name] = entry.content
		total += int64(len(entry.content))
		mode := os.FileMode(0o644)
		if name == upload+"bin/run" && runtime.GOOS != "windows" {
			mode = 0o755
		}
		if entry.mode.Perm() != mode {
			t.Errorf("%s mode = %o, want %o", name, entry.mode.Perm(), mode)
		}
	}
	if !reflect.DeepEqual(contents, want) {
		t.Fatalf("ZIP contents = %#v, want %#v", contents, want)
	}
	if result.FileCount != len(want) || result.TotalSize != total {
		t.Fatalf("result counts = %d/%d, want %d/%d", result.FileCount, result.TotalSize, len(want), total)
	}
	var hashes map[string]string
	if err := json.Unmarshal([]byte(manifest.content), &hashes); err != nil {
		t.Fatal(err)
	}
	wantHashes := make(map[string]string)
	for name, content := range want {
		if rel, ok := strings.CutPrefix(name, "upload/"); ok {
			if strings.HasSuffix(rel, "/.htaccess") || rel == "extra.txt" || rel == addOnPath+"/extra.txt" || strings.HasPrefix(rel, "js/demo/") {
				continue
			}
			digest := sha256.Sum256(bytes.ReplaceAll([]byte(content), []byte{'\r'}, nil))
			wantHashes[rel] = hex.EncodeToString(digest[:])
		}
	}
	if !reflect.DeepEqual(hashes, wantHashes) {
		t.Fatalf("hashes = %#v, want %#v", hashes, wantHashes)
	}
	archiveBytes := readFile(t, f.addon, "_releases/Vendor-Demo-1.0 beta.zip")
	if err := os.Chtimes(filepath.Join(f.addon, "Demo.php"), time.Unix(1000, 0), time.Unix(1000, 0)); err != nil {
		t.Fatal(err)
	}
	second, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
	if err != nil {
		t.Fatal(err)
	}
	if *second != *result {
		t.Fatalf("repeated build result = %+v, want %+v", second, result)
	}
	if got := readFile(t, f.addon, "hashes.json"); got != manifest.content {
		t.Fatal("repeated build changed hashes")
	}
	if got := readFile(t, f.addon, "_releases/Vendor-Demo-1.0 beta.zip"); got != archiveBytes {
		t.Fatal("repeated build or changed source timestamps changed ZIP bytes")
	}
}

// TestBuildTimestamp checks explicit archive timestamps, normalization, and
// rejection of values outside the ZIP timestamp range before output is written.
func TestBuildTimestamp(t *testing.T) {
	t.Parallel()
	for _, timestamp := range []time.Time{
		time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.FixedZone("offset", 2*60*60)),
		time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Unix(1<<32-1, 0),
		{},
		time.Date(1979, time.December, 31, 23, 59, 59, 0, time.UTC),
		time.Unix(1<<32, 0),
	} {
		t.Run(timestamp.String(), func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, `{"whitelist":["addon.json"]}`)
			writeFile(t, f.addon, "_no_upload/README.txt", "instructions")
			result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall(), build.WithTimestamp(timestamp))
			if timestamp.Year() < 1980 || timestamp.Unix() > 1<<32-1 {
				requireError(t, err, "archive timestamp must be between")
				requireMissing(t, f.addon, "hashes.json")
				requireMissing(t, f.addon, "_releases/Vendor-Demo-1.0.zip")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for name, entry := range readArchive(t, result.ZipPath) {
				if want := timestamp.UTC().Truncate(time.Second); !entry.modified.Equal(want) {
					t.Errorf("%s timestamp = %s, want %s", name, entry.modified, want)
				}
			}
		})
	}
}

// TestBuildConfiguredRoots checks installation-relative archive paths and _files
// lookup across supported build roots.
func TestBuildConfiguredRoots(t *testing.T) {
	t.Parallel()
	for _, root := range []string{".", "..", "../..", "../../..", "../../../.."} {
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			prefix := filepath.ToSlash(filepath.Clean(filepath.Join(addOnPath, root)))
			addon, err := filepath.Rel(filepath.FromSlash(prefix), filepath.FromSlash(addOnPath))
			if err != nil {
				t.Fatal(err)
			}
			config, err := json.Marshal(map[string]any{
				"root":             root,
				"whitelist":        []string{filepath.ToSlash(filepath.Join(addon, "addon.json"))},
				"additional_files": []string{"extra.txt", "preferred.txt"},
				"hash_exclude":     []string{"extra.txt", "preferred.txt"},
			})
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, string(config))
			writeFile(t, filepath.Join(f.root, filepath.FromSlash(prefix)), "extra.txt", "extra")
			writeFile(t, filepath.Join(f.root, filepath.FromSlash(prefix)), "preferred.txt", "fallback")
			writeFile(t, f.addon, "_files/preferred.txt", "wrong root")
			writeFile(t, filepath.Join(f.root, filepath.FromSlash(prefix)), "_files/preferred.txt", "preferred")
			result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
			if err != nil {
				t.Fatal(err)
			}
			entries := readArchive(t, result.ZipPath)
			key := "upload/" + strings.TrimPrefix(prefix+"/extra.txt", "./")
			if entries[key].content != "extra" {
				t.Fatalf("missing additional file at %q: %#v", key, entries)
			}
			if entries["upload/preferred.txt"].content != "preferred" {
				t.Fatal("additional file did not use the build root's _files directory")
			}
			var hashes map[string]string
			if err := json.Unmarshal([]byte(entries["upload/"+addOnPath+"/hashes.json"].content), &hashes); err != nil {
				t.Fatal(err)
			}
			if len(hashes) != 1 || hashes[addOnPath+"/addon.json"] == "" {
				t.Fatalf("hash-only exclusions removed the wrong files: %#v", hashes)
			}
			if _, ok := entries["upload/"+addOnPath+"/addon.json"]; !ok {
				t.Fatal("missing add-on metadata at its installation path")
			}
		})
	}
}

// TestBuildWarnings checks configuration and export warnings and their write errors.
func TestBuildWarnings(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{}`)
	source := `{"custom":true}`
	digest := md5.Sum([]byte(source))
	metadata, err := json.Marshal(map[string]string{"_file": "demo.json", "hash": hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, f.addon, "_output/api_scopes/demo.json", source)
	writeFile(t, f.addon, "_output/api_scopes/_metadata.jsonl", string(metadata)+"\n")
	var stderr bytes.Buffer
	if _, err := build.Build(t.Context(), f.root, addOnID, build.WithIO(nil, nil, &stderr)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "no whitelist") {
		t.Fatalf("missing warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), `warning: api_scopes: ignoring unknown field "custom"`) {
		t.Fatalf("missing export warning: %q", stderr.String())
	}
	sentinel := errors.New("writer failed")
	_, err = build.Build(t.Context(), f.root, addOnID, build.WithIO(nil, nil, failingWriter{sentinel}))
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want writer error", err)
	}
	writeFile(t, f.addon, "build.json", `{"whitelist":["addon.json"]}`)
	_, err = build.Build(t.Context(), f.root, addOnID, build.WithIO(nil, nil, failingWriter{sentinel}))
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "writing export warning") {
		t.Fatalf("error = %v, want export warning writer error", err)
	}
}

// TestBuildSelectionErrors checks that invalid selections and archive layouts
// are rejected without returning a release.
func TestBuildSelectionErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		config string
		files  map[string]string
		want   string
	}{
		{"metadata not selected", `{"whitelist":["*.php"]}`, nil, "must be included"},
		{"metadata blacklisted", `{"whitelist":["**"],"blacklist":["addon.json"]}`, nil, "must be included"},
		{"missing additional file", `{"additional_files":["missing.txt"]}`, nil, "reading additional file missing.txt"},
		{"no upload is a file", `{}`, map[string]string{"_no_upload": "bad"}, "_no_upload must be a directory"},
		{"no upload conflict", `{}`, map[string]string{"_no_upload/upload/" + addOnPath + "/addon.json": "bad"}, "conflicts with upload content"},
		{"invalid archive name", `{}`, map[string]string{"bad:name.txt": "bad"}, "invalid archive path"},
		{"data source missing", `{}`, nil, "reading development output"},
		{"data is a file", `{}`, map[string]string{"_data": "bad"}, "_data must be a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.name == "invalid archive name" && runtime.GOOS == "windows" {
				t.Skip("Windows rejects colons in filenames")
			}
			f := newFixture(t, tc.config)
			if strings.HasPrefix(tc.name, "data ") {
				name := "_data"
				if tc.name == "data source missing" {
					name = "_output"
				}
				if err := os.Remove(filepath.Join(f.addon, name)); err != nil {
					t.Fatal(err)
				}
				if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
					t.Fatalf("JavaScript-only build required data export inputs: %v", err)
				}
			}
			for name, content := range tc.files {
				writeFile(t, f.addon, name, content)
			}
			result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
			requireError(t, err, tc.want)
			if result != nil {
				t.Fatalf("failed build returned result: %+v", result)
			}
			requireMissing(t, f.addon, "_releases/Vendor-Demo-1.0.zip")
			if strings.HasPrefix(tc.name, "data ") {
				requireMissing(t, f.addon, "hashes.json")
			}
		})
	}
}

// TestBuildComposerAutoload checks that the declared Composer directory contains
// an installed.json selected for the release, including additional-file overrides.
func TestBuildComposerAutoload(t *testing.T) {
	t.Parallel()
	for _, root := range []string{".", "../../../.."} {
		for _, kind := range []string{"selected", "missing", "blacklisted", "not whitelisted", "directory", "additional"} {
			t.Run(root+"/"+kind, func(t *testing.T) {
				t.Parallel()
				prefix := filepath.ToSlash(filepath.Clean(filepath.Join(addOnPath, root)))
				addon, err := filepath.Rel(filepath.FromSlash(prefix), filepath.FromSlash(addOnPath))
				if err != nil {
					t.Fatal(err)
				}
				installed := filepath.ToSlash(filepath.Join(addon, "vendor/composer/installed.json"))
				settings := map[string]any{"root": root, "whitelist": []string{"**"}}
				if kind == "blacklisted" {
					settings["blacklist"] = []string{installed}
				}
				if kind == "not whitelisted" || kind == "additional" {
					settings["whitelist"] = []string{filepath.ToSlash(filepath.Join(addon, "addon.json"))}
				}
				if kind == "additional" {
					settings["additional_files"] = []string{addOnPath + "/vendor/composer"}
				}
				data, err := json.Marshal(settings)
				if err != nil {
					t.Fatal(err)
				}
				f := newFixture(t, string(data))
				writeFile(t, f.addon, "addon.json", `{"title":"Demo","version_id":100,"version_string":"1.0","composer_autoload":"vendor/composer"}`)
				switch kind {
				case "missing":
				case "directory":
					if err := os.MkdirAll(filepath.Join(f.addon, "vendor/composer/installed.json"), 0o755); err != nil {
						t.Fatal(err)
					}
				case "additional":
					writeFile(t, filepath.Join(f.root, filepath.FromSlash(prefix)), "_files/"+addOnPath+"/vendor/composer/installed.json", "[]")
				default:
					writeFile(t, f.addon, "vendor/composer/installed.json", "[]")
				}
				result, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
				if kind != "selected" && kind != "additional" {
					requireError(t, err, "composer_autoload requires "+addOnPath+"/vendor/composer/installed.json in the release")
					requireMissing(t, f.addon, "hashes.json")
					requireMissing(t, f.addon, "_releases/Vendor-Demo-1.0.zip")
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				entries := readArchive(t, result.ZipPath)
				if entries["upload/"+addOnPath+"/vendor/composer/installed.json"].content != "[]" {
					t.Fatal("declared Composer installed.json did not ship")
				}
			})
		}
	}
}

// TestBuildOutputFailure checks that failed output replacement leaves no
// temporary files.
func TestBuildOutputFailure(t *testing.T) {
	t.Parallel()
	for _, output := range []string{"hashes.json", "_releases/Vendor-Demo-1.0.zip"} {
		t.Run(output, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, `{"whitelist":["addon.json"]}`)
			if err := os.MkdirAll(filepath.Join(f.addon, filepath.FromSlash(output)), 0o755); err != nil {
				t.Fatal(err)
			}
			_, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
			if err == nil {
				t.Fatal("expected output replacement to fail")
			}
			matches, err := filepath.Glob(filepath.Join(f.addon, filepath.Dir(filepath.FromSlash(output)), ".build-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary outputs remain: %v (%v)", matches, err)
			}
		})
	}
}

// TestPublicAPIsCancellation checks that all entry points preserve errors from
// an already canceled context.
func TestPublicAPIsCancellation(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{}`)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := build.Build(ctx, f.root, addOnID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Build error = %v, want cancellation", err)
	}
	if err := build.BuildJavaScript(ctx, f.root, addOnID); !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildJavaScript error = %v, want cancellation", err)
	}
	if err := build.WatchJavaScript(ctx, f.root, addOnID, build.WatchOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchJavaScript error = %v, want cancellation", err)
	}
}
