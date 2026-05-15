package build_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/build"
)

// TestWatchJavaScriptRebuildsAndRecovers checks source and configuration changes,
// including error recovery, input creation, and deletion.
func TestWatchJavaScriptRebuildsAndRecovers(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"minify":["js/**/*.js"]}`)
	writeFile(t, f.addon, "js/a.js", `console.log("initial");`)
	events := startWatch(t, f)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "js/a.min.js", "initial"))

	writeFile(t, f.addon, "js/a.js", `console.log("changed");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "js/a.min.js", "changed"))

	previous := readFile(t, f.addon, "js/a.min.js")
	writeFile(t, f.addon, "js/a.js", "function {")
	waitForOutput(t, events, "build failed", nil)
	if got := readFile(t, f.addon, "js/a.min.js"); got != previous {
		t.Fatal("failed build replaced the previous output")
	}
	writeFile(t, f.addon, "js/a.js", `console.log("recovered");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "js/a.min.js", "recovered"))

	writeFile(t, f.addon, "js/nested/b.js", `console.log("new source");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "js/nested/b.min.js", "new source"))

	writeFile(t, f.addon, "build.json", `{
		"minify":["js/**/*.js","bundle.js"],
		"rollup":{"bundle.js":["js/a.js","js/nested/b.js"],"combined.js":["bundle.js"]}
	}`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "combined.js", "new source"))
	if got := readFile(t, f.addon, "bundle.min.js"); !strings.Contains(got, "recovered") {
		t.Fatalf("rollup was not minified: %q", got)
	}

	writeFile(t, f.addon, "addon.json", `{"title":"Demo","version_id":200,"version_string":"2.0"}`)
	waitForOutput(t, events, "build complete", nil)
	if err := os.Remove(filepath.Join(f.addon, "js", "nested", "b.js")); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, events, "build failed", nil)
	writeFile(t, f.addon, "js/nested/b.js", `console.log("restored");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "combined.js", "restored"))
}

// TestWatchJavaScriptConfigurationRecovery checks recovery from invalid settings
// and detects creation and deletion of optional build.json.
func TestWatchJavaScriptConfigurationRecovery(t *testing.T) {
	t.Parallel()
	for _, missing := range []bool{false, true} {
		name := "invalid configuration"
		if missing {
			name = "missing configuration"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, `{`)
			writeFile(t, f.addon, "source.js", `console.log("recovered");`)
			if missing {
				if err := os.Remove(filepath.Join(f.addon, "build.json")); err != nil {
					t.Fatal(err)
				}
			}
			events := startWatch(t, f)
			if missing {
				waitForOutput(t, events, "build complete", nil)
				requireMissing(t, f.addon, "source.min.js")
			} else {
				waitForOutput(t, events, "build failed", nil)
			}
			writeFile(t, f.addon, "build.json", `{"minify":["source.js"]}`)
			waitForOutput(t, events, "build complete", outputContains(f.addon, "source.min.js", "recovered"))
			if err := os.Remove(filepath.Join(f.addon, "build.json")); err != nil {
				t.Fatal(err)
			}
			waitForOutput(t, events, "build complete", nil)
		})
	}
}

// TestWatchJavaScriptMissingInputRecovery checks that creating a missing input
// directory and source triggers a successful rebuild.
func TestWatchJavaScriptMissingInputRecovery(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"minify":["bundle.js"],"rollup":{"bundle.js":["js/*.js"]}}`)
	events := startWatch(t, f)
	waitForOutput(t, events, "build failed", nil)
	writeFile(t, f.addon, "js/new.js", `console.log("created");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "bundle.min.js", "created"))
}

// TestWatchJavaScriptDoesNotConsumeItsOutputs checks that a watched rollup does
// not consume its own generated files after a source change.
func TestWatchJavaScriptDoesNotConsumeItsOutputs(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"minify":["bundle.js"],"rollup":{"bundle.js":["*.js"]}}`)
	writeFile(t, f.addon, "source.js", `console.log("initial");`)
	events := startWatch(t, f)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "bundle.min.js", "initial"))
	writeFile(t, f.addon, "source.js", `console.log("updated");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "bundle.min.js", "updated"))
	if got := readFile(t, f.addon, "bundle.js"); got != `console.log("updated");` {
		t.Fatalf("rollup consumed generated output: %q", got)
	}
}

// TestWatchJavaScriptSymlinkRecovery checks recovery when a broken link is
// replaced by a regular source file with the same name.
func TestWatchJavaScriptSymlinkRecovery(t *testing.T) {
	t.Parallel()
	f := newFixture(t, `{"minify":["js/*.js"]}`)
	if err := os.Mkdir(filepath.Join(f.addon, "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(f.addon, "js", "source.js")
	symlink(t, "missing.js", filename)
	events := startWatch(t, f)
	waitForOutput(t, events, "build failed", nil)
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	writeFile(t, f.addon, "js/source.js", `console.log("recovered");`)
	waitForOutput(t, events, "build complete", outputContains(f.addon, "js/source.min.js", "recovered"))
}

// TestWatchJavaScriptWriterFailure checks that output errors stop the watcher
// and retain their cause.
func TestWatchJavaScriptWriterFailure(t *testing.T) {
	t.Parallel()
	for _, failedBuild := range []bool{false, true} {
		name := "stdout"
		if failedBuild {
			name = "stderr"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, `{"minify":["source.js"]}`)
			source := "console.log(1);"
			if failedBuild {
				source = "function {"
			}
			writeFile(t, f.addon, "source.js", source)
			sentinel := errors.New("writer failed")
			options := build.WatchOptions{Stdout: failingWriter{sentinel}, Stderr: failingWriter{sentinel}}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			if err := build.WatchJavaScript(ctx, f.root, addOnID, options); !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want writer error", err)
			}
		})
	}
}

// startWatch starts a watcher and arranges for cancellation and shutdown at
// test cleanup.
func startWatch(t *testing.T, f fixture) <-chan string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	output := &outputRecorder{events: make(chan string, 128)}
	done := make(chan error, 1)
	go func() {
		done <- build.WatchJavaScript(ctx, f.root, addOnID, build.WatchOptions{Stdout: output, Stderr: output})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("watch stopped with %v, want cancellation", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("watch did not stop after cancellation")
		}
	})
	return output.events
}

// outputContains returns a predicate that checks a file beneath root for want.
func outputContains(root, name, want string) func() bool {
	return func() bool {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		return err == nil && strings.Contains(string(data), want)
	}
}
