package build_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/build"
)

// TestBuildJavaScriptMinify checks wildcard and overlapping-pattern minification,
// excluding non-JavaScript files and existing minified sources.
func TestBuildJavaScriptMinify(t *testing.T) {
	t.Parallel()
	for _, config := range []string{
		`{"minify":"*"}`,
		`{"minify":["./js/**/*.js","js/*.js","js/**"]}`,
	} {
		t.Run(config, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, config)
			writeFile(t, f.addon, "js/demo.js", "// comment\nconsole.log(1 + 2);\n")
			writeFile(t, f.addon, "js/nested/other.js", "console.log('nested');\n")
			writeFile(t, f.addon, "js/already.min.js", "keep this unchanged")
			writeFile(t, f.addon, "js/readme.txt", "not JavaScript")
			if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
				t.Fatal(err)
			}
			got := readFile(t, f.addon, "js/demo.min.js")
			if !strings.Contains(got, "console.log(3)") || strings.Contains(got, "comment") {
				t.Fatalf("unexpected minified JavaScript: %q", got)
			}
			if !strings.HasSuffix(got, "//# sourceMappingURL=demo.min.js.map\n") {
				t.Fatalf("minified JavaScript does not link its source map: %q", got)
			}
			sourceMap := readFile(t, f.addon, "js/demo.min.js.map")
			if !strings.Contains(sourceMap, `"version": 3`) || !strings.Contains(sourceMap, `"mappings"`) {
				t.Fatalf("unexpected source map: %q", sourceMap)
			}
			if strings.Contains(sourceMap, "sourcesContent") || strings.Contains(sourceMap, "console.log") {
				t.Fatalf("source map embeds source contents: %q", sourceMap)
			}
			if got := readFile(t, f.addon, "js/nested/other.min.js"); !strings.Contains(got, "nested") {
				t.Fatalf("nested source was not minified: %q", got)
			}
			if got := readFile(t, f.addon, "js/already.min.js"); got != "keep this unchanged" {
				t.Fatal("already-minified source was changed")
			}
			requireMissing(t, f.addon, "js/already.min.min.js")
			requireMissing(t, f.addon, "js/readme.txt.min.js")
			requireMissing(t, f.addon, "_build")
			requireMissing(t, f.addon, "_releases")
		})
	}
}

// TestBuildJavaScriptNoTasks checks that empty task settings produce no outputs.
func TestBuildJavaScriptNoTasks(t *testing.T) {
	t.Parallel()
	for _, config := range []string{`{}`, `{"minify":null,"rollup":null}`, `{"minify":[],"rollup":{}}`} {
		t.Run(config, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, config)
			writeFile(t, f.addon, "demo.js", "invalid JavaScript {")
			if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
				t.Fatal(err)
			}
			requireMissing(t, f.addon, "demo.min.js")
		})
	}
}

// TestBuildJavaScriptFilePrecedence checks that _files sources take precedence
// over installation sources.
func TestBuildJavaScriptFilePrecedence(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"root":"../../../..","minify":["js/**/*.js","js/*.js"]}`)
	writeFile(t, f.root, "js/demo.js", `console.log("fallback");`)
	writeFile(t, f.root, "_files/js/demo.js", `console.log("preferred");`)
	writeFile(t, f.root, "js/other.js", `console.log("other");`)
	if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f.root, "_files/js/demo.min.js"); !strings.Contains(got, "preferred") {
		t.Fatalf("wrong source was minified: %q", got)
	}
	requireMissing(t, f.root, "js/demo.min.js")
	if got := readFile(t, f.root, "js/other.min.js"); !strings.Contains(got, "other") {
		t.Fatalf("fallback-only source was not minified: %q", got)
	}
}

// TestBuildJavaScriptRollupPreservesSourceMapComments checks that concatenation
// preserves the input text, including source map links.
func TestBuildJavaScriptRollupPreservesSourceMapComments(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"rollup":{"bundle.js":["a.js","b.js"]}}`)
	writeFile(t, f.addon, "a.js", "console.log('a');\n//# sourceMappingURL=a.min.js.map\n")
	writeFile(t, f.addon, "b.js", "console.log('b');\n//@ sourceMappingURL=b.min.js.map")
	if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
		t.Fatal(err)
	}
	want := "console.log('a');\n//# sourceMappingURL=a.min.js.map\n\n\nconsole.log('b');\n//@ sourceMappingURL=b.min.js.map"
	if got := readFile(t, f.addon, "bundle.js"); got != want {
		t.Fatalf("rollup = %q, want %q", got, want)
	}
}

// TestBuildJavaScriptRollups checks input ordering, generated-output precedence,
// chained rollups, and deferred minification.
func TestBuildJavaScriptRollups(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{
		"root":"../../../..",
		"minify":["js/first.js","js/first.js"],
		"rollup":{
			"js/first.js":["js/b.js","js/a.js"],
			"js/second.js":["js/first.js","js/{b,a}.js"]
		},
		"exec":[["must-not-run"]]
	}`)
	writeFile(t, f.root, "composer.json", `{}`)
	writeFile(t, f.root, "js/a.js", `console.log("fallback");`)
	writeFile(t, f.root, "_files/js/a.js", `console.log("preferred");`)
	writeFile(t, f.addon, "_files/js/a.js", `console.log("wrong root");`)
	writeFile(t, f.root, "js/b.js", `console.log("second");`)
	writeFile(t, f.root, "_files/js/first.js", `console.log("stale shadow");`)
	writeFile(t, f.root, "js/first.js", "invalid stale output {")
	wantFirst := "console.log(\"second\");\n\nconsole.log(\"preferred\");"
	wantSecond := wantFirst + "\n\nconsole.log(\"preferred\");\n\nconsole.log(\"second\");"
	for range 2 {
		if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, f.root, "js/first.js"); got != wantFirst {
			t.Fatalf("first rollup = %q, want %q", got, wantFirst)
		}
		if got := readFile(t, f.root, "js/second.js"); got != wantSecond {
			t.Fatalf("chained rollup = %q, want %q", got, wantSecond)
		}
		if got := readFile(t, f.root, "js/first.min.js"); strings.Contains(got, "stale") || !strings.Contains(got, "preferred") {
			t.Fatalf("wrong rollup was minified: %q", got)
		}
	}
	requireMissing(t, f.root, "composer.txt")
	requireMissing(t, f.addon, "_build")
}

// TestBuildJavaScriptDoesNotConsumeItsOutputs checks that repeated builds do not
// feed a bundle or its minified companion back into the bundle's inputs.
func TestBuildJavaScriptDoesNotConsumeItsOutputs(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"minify":["bundle.js"],"rollup":{"bundle.js":["*.js"]}}`)
	writeFile(t, f.addon, "a.js", "console.log('a');\n")
	writeFile(t, f.addon, "b.js", "console.log('b');\n")
	var previous string
	for i := range 3 {
		if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
			t.Fatal(err)
		}
		got := readFile(t, f.addon, "bundle.js")
		if i > 0 && got != previous {
			t.Fatalf("rollup changed on build %d: %q -> %q", i+1, previous, got)
		}
		previous = got
	}
}

// TestBuildJavaScriptPreservesUnchangedOutputs checks that unchanged outputs
// retain their file identity and modification time.
func TestBuildJavaScriptPreservesUnchangedOutputs(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"minify":["bundle.js"],"rollup":{"bundle.js":["source.js"]}}`)
	writeFile(t, f.addon, "source.js", "console.log('source');\n")
	if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
		t.Fatal(err)
	}
	before := make(map[string]os.FileInfo)
	for _, name := range []string{"bundle.js", "bundle.min.js"} {
		filename := filepath.Join(f.addon, name)
		if err := os.Chtimes(filename, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = info
	}
	if err := build.BuildJavaScript(t.Context(), f.root, addOnID); err != nil {
		t.Fatal(err)
	}
	for name, info := range before {
		after, err := os.Stat(filepath.Join(f.addon, name))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) {
			t.Errorf("unchanged output %s was rewritten", name)
		}
	}
}

// TestBuildJavaScriptErrors checks invalid sources, missing inputs, and output
// failures, including temporary-file cleanup.
func TestBuildJavaScriptErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		config    string
		source    string
		outputDir string
		want      string
	}{
		{"invalid source", `{"minify":["source.js"]}`, "function {", "", "minifying source.js"},
		{"invalid rollup", `{"minify":["bundle.js"],"rollup":{"bundle.js":["source.js"]}}`, "function {", "", "minifying bundle.js"},
		{"missing input", `{"rollup":{"bundle.js":["missing.js"]}}`, "console.log(1);", "", "matched no files"},
		{"minified output directory", `{"minify":["source.js"]}`, "console.log(1);", "source.min.js", "not a regular file"},
		{"rollup output directory", `{"rollup":{"bundle.js":["source.js"]}}`, "console.log(1);", "bundle.js", "not a regular file"},
		{"output parent is a file", `{"rollup":{"source.js/bundle.js":["source.js"]}}`, "console.log(1);", "", "writing rollup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc.config)
			writeFile(t, f.addon, "source.js", tc.source)
			if tc.outputDir != "" {
				if err := os.Mkdir(filepath.Join(f.addon, tc.outputDir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			requireError(t, build.BuildJavaScript(t.Context(), f.root, addOnID), tc.want)
			matches, err := filepath.Glob(filepath.Join(f.addon, ".build-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary outputs remain: %v (%v)", matches, err)
			}
		})
	}
}
