package build_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/build"
)

// TestMain runs the suite or acts as a helper subprocess when requested.
func TestMain(m *testing.M) {
	if os.Getenv("XF_BUILD_TEST_COMMAND") == "1" {
		if err := runTestCommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(23)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestBuildCommands checks command ordering, placeholders, working directories,
// standard streams, and inclusion of command-generated files.
func TestBuildCommands(t *testing.T) {
	executable, log := commandHelper(t)
	installTool(t, executable, "composer")
	t.Setenv("XF_BUILD_COMPOSER_INSTALLED", addOnPath+"/vendor/composer/installed.json")
	config, err := json.Marshal(map[string]any{
		"root":      "../../../..",
		"whitelist": []string{addOnPath + "/**", "composer.txt"},
		"exec":      [][]string{{executable, "record", "{addon_id}", "{version_string}", "{version_id}", "{enabled}", "{empty}", "{missing}", "{nested}", "two words"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, string(config))
	writeFile(t, f.addon, "addon.json", `{"title":"Demo","version_string":"1.0","version_id":9007199254740993,"composer_autoload":"vendor/composer","require":[],"extra_urls":{},"enabled":false,"empty":"","nested":{}}`)
	writeFile(t, f.root, "composer.json", `{}`)
	var stdout, stderr bytes.Buffer
	result, err := build.Build(t.Context(), f.root, addOnID, build.WithIO(strings.NewReader("command input"), &stdout, &stderr))
	if err != nil {
		t.Fatal(err)
	}
	records := readCommands(t, log)
	if len(records) != 2 {
		t.Fatalf("commands = %+v, want Composer then exec", records)
	}
	wantComposer := []string{"install", "--no-dev", "--no-interaction", "--no-progress"}
	if !reflect.DeepEqual(records[0].Args, wantComposer) {
		t.Fatalf("Composer arguments = %q, want %q", records[0].Args, wantComposer)
	}
	wantExec := []string{"record", addOnID, "1.0", "9007199254740993", "false", "{empty}", "{missing}", "{nested}", "two words"}
	if !reflect.DeepEqual(records[1].Args, wantExec) {
		t.Fatalf("exec arguments = %q, want %q", records[1].Args, wantExec)
	}
	for i, dir := range []string{f.root, f.addon} {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if records[i].Dir != real {
			t.Errorf("command %d directory = %q, want %q", i, records[i].Dir, real)
		}
	}
	if records[0].Input != "command input" || records[1].Input != "" {
		t.Fatalf("stdin was not forwarded: %+v", records)
	}
	if stdout.String() != "command stdout\ncommand stdout\n" || stderr.String() != "command stderr\ncommand stderr\n" {
		t.Fatalf("command streams = %q / %q", stdout.String(), stderr.String())
	}
	entries := readArchive(t, result.ZipPath)
	if entries["upload/composer.txt"].content != "installed" || entries["upload/"+addOnPath+"/generated.txt"].content != "generated" {
		t.Fatal("command-generated files were not included in the release")
	}
	if entries["upload/"+addOnPath+"/vendor/composer/installed.json"].content != "[]" {
		t.Fatal("Composer autoload validation did not wait for dependency installation")
	}
}

// TestBuildExecForms checks string commands, argument arrays, and mixed lists.
func TestBuildExecForms(t *testing.T) {
	stringCommand := " \tbuild-helper  record\t{addon_id} \n"
	arrayCommand := []string{"build-helper", "record", "two words"}
	for _, tc := range []struct {
		name string
		exec []any
		want [][]string
	}{
		{"strings", []any{stringCommand}, [][]string{{"record", addOnID}}},
		{"arrays", []any{arrayCommand}, [][]string{{"record", "two words"}}},
		{"mixed", []any{stringCommand, arrayCommand}, [][]string{{"record", addOnID}, {"record", "two words"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executable, log := commandHelper(t)
			installTool(t, executable, "build-helper")
			config, err := json.Marshal(map[string]any{"exec": tc.exec})
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, string(config))
			if _, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall()); err != nil {
				t.Fatal(err)
			}
			records := readCommands(t, log)
			if len(records) != len(tc.want) {
				t.Fatalf("commands = %+v, want %d invocations", records, len(tc.want))
			}
			for i, want := range tc.want {
				if !slices.Equal(records[i].Args, want) {
					t.Errorf("command %d arguments = %q, want %q", i, records[i].Args, want)
				}
			}
		})
	}
}

// TestBuildSkipsDependencyInstall checks that skipping Composer still runs
// configured commands.
func TestBuildSkipsDependencyInstall(t *testing.T) {
	executable, log := commandHelper(t)
	installTool(t, executable, "composer")
	config, err := json.Marshal(map[string]any{
		"whitelist": []string{"**"},
		"exec":      [][]string{{executable, "record"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, string(config))
	writeFile(t, f.addon, "composer.json", `{}`)
	if _, err := build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall()); err != nil {
		t.Fatal(err)
	}
	records := readCommands(t, log)
	if len(records) != 1 || records[0].Args[0] != "record" {
		t.Fatalf("commands = %+v, want exec only", records)
	}
	requireMissing(t, f.addon, "composer.txt")
}

// TestBuildCommandFailure checks exit status propagation and that a failed
// command prevents later commands and release generation.
func TestBuildCommandFailure(t *testing.T) {
	executable, _ := commandHelper(t)
	config, err := json.Marshal(map[string]any{
		"whitelist": []string{"addon.json"},
		"exec":      [][]string{{executable, "fail"}, {executable, "record"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, string(config))
	_, err = build.Build(t.Context(), f.root, addOnID, build.WithoutDependencyInstall())
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("error = %v, want exit status 23", err)
	}
	requireError(t, err, "exec ")
	requireMissing(t, f.addon, "generated.txt")
	requireMissing(t, f.addon, "hashes.json")
}

// TestBuildComposerFailures checks an unavailable executable and an escaping
// composer.json link.
func TestBuildComposerFailures(t *testing.T) {
	for _, kind := range []string{"missing executable", "invalid composer file"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, `{"whitelist":["addon.json"]}`)
			writeFile(t, f.addon, "composer.json", `{}`)
			want := "composer install"
			if kind == "missing executable" {
				t.Setenv("PATH", t.TempDir())
			} else {
				if err := os.Remove(filepath.Join(f.addon, "composer.json")); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(t.TempDir(), "composer.json")
				writeFile(t, filepath.Dir(outside), "composer.json", `{}`)
				symlink(t, outside, filepath.Join(f.addon, "composer.json"))
				want = "reading composer.json"
			}
			_, err := build.Build(t.Context(), f.root, addOnID)
			requireError(t, err, want)
		})
	}
}

// TestBuildComposerRouting checks environment selection and build-root handling,
// while configured exec commands continue to run on the host.
func TestBuildComposerRouting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		root   string
		mode   string
		action string
	}{
		{"host add-on", ".", "", ""},
		{"host installation", "../../../..", "", ""},
		{"running add-on", ".", "running", "exec"},
		{"stopped parent", "../..", "stopped", "run"},
		{"service stops before exec", ".", "not-running", "run"},
		{"running installation", "../../../..", "running", "exec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executable, log := commandHelper(t)
			installTool(t, executable, "composer")
			installTool(t, executable, "docker")
			t.Setenv("XF_BUILD_DOCKER_MODE", tc.mode)
			config, err := json.Marshal(map[string]any{
				"root":      tc.root,
				"whitelist": []string{"**"},
				"exec":      [][]string{{executable, "record"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, string(config))
			project := filepath.ToSlash(filepath.Clean(filepath.Join(addOnPath, tc.root)))
			working := filepath.Join(f.root, filepath.FromSlash(project))
			writeFile(t, working, "composer.json", `{}`)
			if tc.mode != "" {
				writeFile(t, f.root, "compose.yaml", "services: {}\n")
				writeFile(t, f.root, ".env", "XF_INSTANCE=build-demo\n")
			}
			var stdout, stderr bytes.Buffer
			result, err := build.Build(t.Context(), f.root, addOnID, build.WithIO(strings.NewReader("input"), &stdout, &stderr))
			if err != nil {
				t.Fatal(err)
			}
			realRoot, err := filepath.EvalSymlinks(f.root)
			if err != nil {
				t.Fatal(err)
			}
			realAddon, err := filepath.EvalSymlinks(f.addon)
			if err != nil {
				t.Fatal(err)
			}
			records := readCommands(t, log)
			count := 2
			if tc.mode != "" {
				count++
			}
			if tc.mode == "not-running" {
				count++
			}
			if len(records) != count {
				t.Fatalf("commands = %+v, want %d invocations", records, count)
			}
			last := records[len(records)-1]
			if !reflect.DeepEqual(last.Args, []string{"record"}) || last.Dir != realAddon {
				t.Fatalf("exec did not run on the host in the add-on directory: %+v", last)
			}
			composer := records[len(records)-2]
			want := []string{"install", "--no-dev", "--no-interaction", "--no-progress"}
			if project != "." {
				want = append(want, "--working-dir="+project)
			}
			if tc.mode != "" {
				prefix := []string{"compose", "--project-name", "build-demo", "--file", filepath.Join(realRoot, "compose.yaml")}
				probe := append(slices.Clone(prefix), "ps", "--status", "running", "--services", "xf")
				if !reflect.DeepEqual(records[0].Args, probe) {
					t.Fatalf("environment probe = %q, want %q", records[0].Args, probe)
				}
				prefix = append(prefix, tc.action)
				if tc.action == "run" {
					prefix = append(prefix, "--rm")
				} else {
					prefix = append(prefix, "-T")
				}
				want = append(append(prefix, "xf", "composer"), want...)
			}
			if !reflect.DeepEqual(composer.Args, want) || composer.Dir != realRoot {
				t.Fatalf("Composer invocation = %+v, want %q in %q", composer, want, realRoot)
			}
			if tc.mode != "not-running" && composer.Input != "input" {
				t.Fatalf("Composer stdin = %q, want input", composer.Input)
			}
			if stdout.String() != "command stdout\ncommand stdout\n" || stderr.String() != "command stderr\ncommand stderr\n" {
				t.Fatalf("command streams = %q / %q", stdout.String(), stderr.String())
			}
			entries := readArchive(t, result.ZipPath)
			composerPath := "upload/" + strings.TrimPrefix(project+"/composer.txt", "./")
			if entries[composerPath].content != "installed" || entries["upload/"+addOnPath+"/generated.txt"].content != "generated" {
				t.Fatal("generated files were not included at their installation paths")
			}
		})
	}
}

// TestBuildComposerEnvironmentFailures checks that Docker and container command
// failures never retry Composer on the host.
func TestBuildComposerEnvironmentFailures(t *testing.T) {
	for _, mode := range []string{"unavailable", "command-error"} {
		t.Run(mode, func(t *testing.T) {
			executable, log := commandHelper(t)
			installTool(t, executable, "composer")
			installTool(t, executable, "docker")
			t.Setenv("XF_BUILD_DOCKER_MODE", mode)
			f := newFixture(t, `{"whitelist":["addon.json"]}`)
			writeFile(t, f.addon, "composer.json", `{}`)
			writeFile(t, f.root, "compose.yaml", "services: {}\n")
			result, err := build.Build(t.Context(), f.root, addOnID)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 || result != nil {
				t.Fatalf("result = %+v, error = %v, want Docker exit status 23", result, err)
			}
			for _, record := range readCommands(t, log) {
				if record.Args[0] != "compose" {
					t.Fatalf("retried command on host: %+v", record)
				}
			}
			requireMissing(t, f.addon, "composer.txt")
			requireMissing(t, f.addon, "hashes.json")
		})
	}
}

// TestBuildComposerOutputFailure checks that environment commands preserve
// errors from the caller's output writer.
func TestBuildComposerOutputFailure(t *testing.T) {
	for _, mode := range []string{"running", "stopped"} {
		t.Run(mode, func(t *testing.T) {
			executable, _ := commandHelper(t)
			installTool(t, executable, "docker")
			t.Setenv("XF_BUILD_DOCKER_MODE", mode)
			f := newFixture(t, `{"whitelist":["addon.json"]}`)
			writeFile(t, f.addon, "composer.json", `{}`)
			writeFile(t, f.root, "compose.yaml", "services: {}\n")
			sentinel := errors.New("writer failed")
			_, err := build.Build(t.Context(), f.root, addOnID, build.WithIO(nil, nil, failingWriter{sentinel}))
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want output writer error", err)
			}
		})
	}
}

// TestBuildCommandCancellation checks cancellation after host and environment
// commands start, including shutdown of the build goroutine.
func TestBuildCommandCancellation(t *testing.T) {
	for _, kind := range []string{"exec", "host Composer", "environment Composer"} {
		t.Run(kind, func(t *testing.T) {
			executable, _ := commandHelper(t)
			config := map[string]any{"whitelist": []string{"addon.json"}}
			if kind == "exec" {
				config["exec"] = [][]string{{executable, "wait"}}
			}
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, string(data))
			if kind != "exec" {
				installTool(t, executable, "composer")
				t.Setenv("XF_BUILD_COMPOSER_WAIT", "1")
				writeFile(t, f.addon, "composer.json", `{}`)
			}
			if kind == "environment Composer" {
				installTool(t, executable, "docker")
				writeFile(t, f.root, "compose.yaml", "services: {}\n")
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			output := &outputRecorder{events: make(chan string, 32)}
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				_, err := build.Build(ctx, f.root, addOnID, build.WithIO(nil, output, nil))
				done <- err
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-finished:
				case <-time.After(10 * time.Second):
					t.Error("command did not stop")
				}
			})
			waitForOutput(t, output.events, "ready", nil)
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v, want cancellation", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("build did not stop after cancellation")
			}
		})
	}
}

// commandRecord captures the arguments, working directory, and input seen by a
// helper subprocess.
type commandRecord struct {
	Args  []string
	Dir   string
	Input string
}

// commandHelper enables subprocess mode and returns the test executable and a
// log path. The calling test must not run in parallel.
func commandHelper(t *testing.T) (executable, log string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log = filepath.Join(t.TempDir(), "commands.jsonl")
	t.Setenv("XF_BUILD_TEST_COMMAND", "1")
	t.Setenv("XF_BUILD_COMMAND_LOG", log)
	return executable, log
}

// installTool puts the test executable on PATH under name.
func installTool(t *testing.T, executable, name string) {
	t.Helper()
	bin := t.TempDir()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), data, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// runTestCommand records a helper invocation and performs the action named by
// its first argument.
func runTestCommand() error {
	args := os.Args[1:]
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(os.Getenv("XF_BUILD_COMMAND_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(log).Encode(commandRecord{Args: args, Dir: dir, Input: string(input)})
	closeErr := log.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	working := dir
	for _, argument := range args {
		if project, ok := strings.CutPrefix(argument, "--working-dir="); ok {
			working = filepath.Join(dir, filepath.FromSlash(project))
			break
		}
	}
	switch args[0] {
	case "compose":
		mode := os.Getenv("XF_BUILD_DOCKER_MODE")
		if slices.Contains(args, "ps") {
			if mode == "unavailable" {
				return fmt.Errorf("Docker unavailable")
			}
			if mode == "stopped" {
				return nil
			}
			_, err := fmt.Fprintln(os.Stdout, "xf")
			return err
		}
		if mode == "not-running" && slices.Contains(args, "exec") {
			return fmt.Errorf("service xf is not running")
		}
		if mode == "command-error" {
			return fmt.Errorf("Composer failed")
		}
		if err := os.WriteFile(filepath.Join(working, "composer.txt"), []byte("installed"), 0o644); err != nil {
			return err
		}
	case "install":
		if err := os.WriteFile(filepath.Join(working, "composer.txt"), []byte("installed"), 0o644); err != nil {
			return err
		}
	case "record":
		if err := os.WriteFile("generated.txt", []byte("generated"), 0o644); err != nil {
			return err
		}
	case "fail":
		return fmt.Errorf("command failed")
	}
	if args[0] == "install" || args[0] == "compose" {
		if name := os.Getenv("XF_BUILD_COMPOSER_INSTALLED"); name != "" {
			filename := filepath.Join(working, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filename, []byte("[]"), 0o644); err != nil {
				return err
			}
		}
	}
	if args[0] == "wait" || os.Getenv("XF_BUILD_COMPOSER_WAIT") == "1" {
		if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
			return err
		}
		time.Sleep(time.Hour)
	}
	if _, err := fmt.Fprintln(os.Stdout, "command stdout"); err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stderr, "command stderr")
	return err
}

// readCommands reads recorded helper invocations in execution order.
func readCommands(t *testing.T, filename string) []commandRecord {
	t.Helper()
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var records []commandRecord
	decoder := json.NewDecoder(f)
	for {
		var record commandRecord
		if err := decoder.Decode(&record); err == io.EOF {
			return records
		} else if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
}
