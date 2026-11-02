package build_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/build"
)

func put(t *testing.T, root, name, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, config string) (string, string) {
	t.Helper()
	root := t.TempDir()
	addon := filepath.Join(root, "src/addons/Acme/Demo")
	put(t, addon, "addon.json", `{"version_string":"1.2 Beta"}`)
	put(t, addon, "build.json", config)
	if err := os.MkdirAll(filepath.Join(addon, "_output"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, addon
}

func TestBuildReleaseContentsAndReproducibility(t *testing.T) {
	root, addon := fixture(t, `{"whitelist":["keep.txt","skip.txt","unhashed.txt"],"blacklist":["skip.txt"],"additional_files":["keep.txt"],"hash_exclude":["unhashed.txt"]}`)
	put(t, addon, "keep.txt", "original")
	put(t, addon, "_files/keep.txt", "replacement\r\nline\r\n")
	put(t, addon, "skip.txt", "excluded")
	put(t, addon, "unhashed.txt", "included without hash")
	put(t, addon, "_no_upload/README.txt", "instructions")
	stamp := time.Date(2024, 2, 3, 4, 5, 6, 999, time.FixedZone("offset", 3600))
	opts := []build.Option{build.WithoutDependencyInstall(), build.WithTimestamp(stamp)}
	result, err := build.Build(context.Background(), root, "Acme/Demo", opts...)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(result.ZipPath) != "Acme-Demo-1.2-Beta.zip" {
		t.Fatalf("release path: %s", result.ZipPath)
	}
	first, err := os.ReadFile(result.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, file := range zr.File {
		if !file.Modified.Equal(stamp.UTC().Truncate(time.Second)) {
			t.Errorf("%s timestamp: %s", file.Name, file.Modified)
		}
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		contents[file.Name] = string(data)
	}
	prefix := "upload/src/addons/Acme/Demo/"
	manifestData, ok := contents[prefix+"hashes.json"]
	if !ok {
		t.Fatal("missing manifest")
	}
	delete(contents, prefix+"hashes.json")
	want := map[string]string{"README.txt": "instructions", "upload/keep.txt": "replacement\r\nline\r\n", prefix + "addon.json": `{"version_string":"1.2 Beta"}`, prefix + "keep.txt": "original", prefix + "unhashed.txt": "included without hash"}
	if !reflect.DeepEqual(contents, want) {
		t.Fatalf("archive contents: %#v", contents)
	}
	var manifest map[string]string
	if err := json.Unmarshal([]byte(manifestData), &manifest); err != nil {
		t.Fatal(err)
	}
	digest := func(s string) string {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(s, "\r", ""))))
	}
	expected := map[string]string{"keep.txt": digest(want["upload/keep.txt"]), "src/addons/Acme/Demo/addon.json": digest(want[prefix+"addon.json"]), "src/addons/Acme/Demo/keep.txt": digest(want[prefix+"keep.txt"])}
	if !reflect.DeepEqual(manifest, expected) {
		t.Fatalf("manifest: %#v", manifest)
	}
	diskManifest, err := os.ReadFile(filepath.Join(addon, "hashes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(diskManifest) != manifestData {
		t.Fatal("archive and disk manifests differ")
	}
	var size int64
	for _, data := range want {
		size += int64(len(data))
	}
	if result.FileCount != len(want) || result.TotalSize != size {
		t.Fatalf("counts: %d files, %d bytes", result.FileCount, result.TotalSize)
	}
	second, err := build.Build(context.Background(), root, "Acme/Demo", opts...)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := os.ReadFile(second.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, rebuilt) {
		t.Fatal("fixed-timestamp archives differ")
	}
}

func TestBuildTimestampBoundaries(t *testing.T) {
	minimum := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	maximum := time.Unix(1<<32-1, 0).UTC()
	for _, tc := range []struct {
		name      string
		timestamp time.Time
		valid     bool
	}{
		{"minimum", minimum, true},
		{"maximum", maximum, true},
		{"before minimum", minimum.Add(-time.Second), false},
		{"after maximum", maximum.Add(time.Second), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := fixture(t, `{}`)
			result, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall(), build.WithTimestamp(tc.timestamp))
			if !tc.valid {
				if err == nil {
					t.Fatal("accepted out-of-range timestamp")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			archive, err := zip.OpenReader(result.ZipPath)
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			if len(archive.File) == 0 {
				t.Fatal("empty archive")
			}
			for _, entry := range archive.File {
				if !entry.Modified.Equal(tc.timestamp) {
					t.Errorf("%s timestamp: %s", entry.Name, entry.Modified)
				}
			}
		})
	}
}

func TestBuildRejectsUnsafeAndFailedInputs(t *testing.T) {
	t.Run("invalid add-on ID", func(t *testing.T) {
		root, _ := fixture(t, `{}`)
		if _, err := build.Build(context.Background(), root, "../Demo", build.WithoutDependencyInstall()); err == nil {
			t.Fatal("accepted invalid add-on ID")
		}
	})
	t.Run("malformed config", func(t *testing.T) {
		root, _ := fixture(t, `{"whitelist":`)
		if _, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall()); err == nil {
			t.Fatal("accepted malformed build.json")
		}
	})
	t.Run("traversal", func(t *testing.T) {
		root, _ := fixture(t, `{"additional_files":["../secret"]}`)
		if _, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall()); err == nil {
			t.Fatal("accepted traversal")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		root, addon := fixture(t, `{"whitelist":["leak.txt"]}`)
		outside := filepath.Join(t.TempDir(), "secret")
		if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(addon, "leak.txt")); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symbolic links unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall()); err == nil {
			t.Fatal("accepted external symlink")
		}
	})
	t.Run("missing additional file", func(t *testing.T) {
		root, addon := fixture(t, `{"additional_files":["missing.txt"]}`)
		if _, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall()); err == nil {
			t.Fatal("accepted missing input")
		}
		if _, err := os.Stat(filepath.Join(addon, "_releases")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected release: %v", err)
		}
	})
	t.Run("javascript task failure", func(t *testing.T) {
		root, addon := fixture(t, `{"minify":["js/broken.js"]}`)
		put(t, addon, "js/broken.js", "function {")
		if _, err := build.Build(context.Background(), root, "Acme/Demo", build.WithoutDependencyInstall()); err == nil {
			t.Fatal("accepted invalid JavaScript")
		}
		if _, err := os.Stat(filepath.Join(addon, "_releases")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected release: %v", err)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		root, _ := fixture(t, `{}`)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := build.Build(ctx, root, "Acme/Demo", build.WithoutDependencyInstall()); !errors.Is(err, context.Canceled) {
			t.Fatalf("error: %v", err)
		}
	})
}

func TestBuildJavaScriptRollsUpAndMinifies(t *testing.T) {
	root, addon := fixture(t, `{"rollup":{"js/bundle.js":["js/one.js","js/two.js"]},"minify":["js/bundle.js"]}`)
	put(t, addon, "js/one.js", "window.first = 1;")
	put(t, addon, "js/two.js", "window.second = 2;")
	if err := build.BuildJavaScript(context.Background(), root, "Acme/Demo"); err != nil {
		t.Fatal(err)
	}
	bundle, err := os.ReadFile(filepath.Join(addon, "js/bundle.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), "window.first = 1;") || !strings.Contains(string(bundle), "window.second = 2;") || bytes.Index(bundle, []byte("first")) > bytes.Index(bundle, []byte("second")) {
		t.Fatalf("bundle: %s", bundle)
	}
	minified, err := os.ReadFile(filepath.Join(addon, "js/bundle.min.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`window\s*\.\s*first\s*=\s*1\b`).Match(minified) ||
		!regexp.MustCompile(`window\s*\.\s*second\s*=\s*2\b`).Match(minified) ||
		!bytes.Contains(minified, []byte("sourceMappingURL=bundle.min.js.map")) {
		t.Fatalf("minified: %s", minified)
	}
	var sourceMap map[string]any
	data, err := os.ReadFile(filepath.Join(addon, "js/bundle.min.js.map"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &sourceMap); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sourceMap["sources"], []any{"bundle.js"}) {
		t.Fatalf("source map: %s", data)
	}
}
