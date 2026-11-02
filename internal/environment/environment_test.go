package environment_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/environment"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func installation(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "My Forum")
	writeFile(t, filepath.Join(dir, "src", "XF.php"), "<?php")
	return dir
}

func TestInitPreservesUserFilesAndOverwritePolicy(t *testing.T) {
	dir := installation(t)
	if _, err := environment.Init(dir, environment.InitOptions{}); err != nil {
		t.Fatal(err)
	}
	if written, err := environment.Init(dir, environment.InitOptions{}); err != nil || len(written) != 1 || filepath.Base(written[0]) != ".env.default" {
		t.Fatalf("second initialization: %v, %v; want only the .env default companion", written, err)
	}
	compose := filepath.Join(dir, "compose.yaml")
	ignore := filepath.Join(dir, ".dockerignore")
	env := filepath.Join(dir, ".env")
	defaultCompose := readFile(t, compose)
	writeFile(t, compose, "custom compose\n")
	writeFile(t, ignore, "custom ignore\n")
	writeFile(t, env, "XF_INSTANCE=custom\n")
	written, err := environment.Init(dir, environment.InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, compose); got != "custom compose\n" {
		t.Fatalf("compose overwritten: %q", got)
	}
	if got := readFile(t, ignore); got != "custom ignore\n" {
		t.Fatalf("ignore overwritten: %q", got)
	}
	defaultIgnore, err := environment.GetDockerFile(".dockerignore.default")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, ignore+".default"); got != string(defaultIgnore) {
		t.Fatal("default companion does not match embedded file")
	}
	if !slices.Contains(written, ignore+".default") {
		t.Fatalf("missing companion in written paths: %v", written)
	}
	if _, err := environment.Init(dir, environment.InitOptions{OverwriteExisting: true}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, compose); got != defaultCompose {
		t.Fatal("overwrite did not restore base file")
	}
	if got := readFile(t, ignore); got != "custom ignore\n" {
		t.Fatal("overwrite replaced user default-based file")
	}
	if got := readFile(t, env); got != "XF_INSTANCE=custom\n" {
		t.Fatal("overwrite replaced user environment")
	}
}

func TestEnvFileRoundTripAndPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	writeFile(t, path, "# settings\n  NAME=old\nUNCHANGED='kept value'\nmalformed\n")
	want := map[string]string{"NAME": "new value", "ADDED": "hello world"}
	if err := environment.WriteEnvFile(path, want); err != nil {
		t.Fatal(err)
	}
	content := readFile(t, path)
	for _, part := range []string{"# settings\n", "  NAME=\"new value\"", "UNCHANGED='kept value'", "malformed"} {
		if !strings.Contains(content, part) {
			t.Errorf("%q missing from %q", part, content)
		}
	}
	values, err := environment.ReadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["NAME"] != want["NAME"] || values["ADDED"] != want["ADDED"] || values["UNCHANGED"] != "kept value" {
		t.Fatalf("unexpected values: %v", values)
	}
}

func TestEnvFileCreateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	want := map[string]string{
		"EMPTY": "", "SPACES": "hello world", "HASH": "value#tag",
		"QUOTES": `say "hello"`, "EQUALS": "one=two",
	}
	if err := environment.WriteEnvFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := environment.ReadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: got %v, want %v", got, want)
	}
}

func TestEnvFileCreateIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := environment.WriteEnvFile(path, map[string]string{"NAME": "value"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("mode %o grants group or other access", perm)
	}
}

func TestRunnerComposeAndExecFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := installation(t)
	for _, name := range []string{"compose.yaml", "compose.mysql.yaml", "compose.override.yaml"} {
		writeFile(t, filepath.Join(dir, name), "")
	}
	writeFile(t, filepath.Join(dir, ".env"), "XF_INSTANCE=forum\nXF_CONTEXTS=mysql:missing\n")
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "docker.log")
	writeFile(t, filepath.Join(bin, "docker"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DOCKER_LOG\"\ncase \" $* \" in\n  *' ps '*) echo xf ;;\n  *' exec '*) echo 'container is not running' >&2; exit 1 ;;\n  *' run '*) printf '%s|%s' \"$XF_DIR\" \"$SECRET\" ;;\nesac\n")
	if err := os.Chmod(filepath.Join(bin, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("DOCKER_LOG", log)
	runner, err := environment.NewRunner(dir)
	if err != nil {
		t.Fatal(err)
	}
	if runner.Instance() != "forum" || !reflect.DeepEqual(runner.Contexts(), []string{"mysql", "missing"}) {
		t.Fatal("runner did not read environment")
	}
	var stdout, stderr bytes.Buffer
	if err := runner.ExecOrRun(context.Background(), "xf", map[string]string{"SECRET": "sensitive"}, strings.NewReader(""), &stdout, &stderr, "php", "-v"); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != dir+"|sensitive" || stderr.Len() != 0 {
		t.Fatalf("streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(readFile(t, log)), "\n")
	var sawExec, sawRun bool
	for _, line := range lines {
		sawExec = sawExec || strings.Contains(" "+line+" ", " exec ")
		sawRun = sawRun || strings.Contains(" "+line+" ", " run ")
		for _, part := range []string{"--project-name forum", "--file " + filepath.Join(dir, "compose.mysql.yaml"), "--file " + filepath.Join(dir, "compose.override.yaml")} {
			if !strings.Contains(line, part) {
				t.Errorf("%q missing %q", line, part)
			}
		}
		if strings.Contains(line, "compose.missing.yaml") || strings.Contains(line, "sensitive") {
			t.Errorf("unexpected context or secret in arguments: %q", line)
		}
	}
	if !sawExec || !sawRun {
		t.Fatalf("expected exec and run fallback in Docker calls: %v", lines)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.Compose(ctx, nil, &stdout, &stderr, "ps"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestRunInDirFallsBackToLocalCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := installation(t)
	work := filepath.Join(dir, "nested")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := environment.RunInDir(context.Background(), dir, "nested", strings.NewReader("input"), &stdout, &stderr, "/bin/sh", "-c", "pwd; cat"); err != nil {
		t.Fatal(err)
	}
	outputDir, input, ok := strings.Cut(stdout.String(), "\n")
	if !ok || input != "input" {
		t.Fatalf("local command output: %q", stdout.String())
	}
	wantDir, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, err := filepath.EvalSymlinks(outputDir)
	if err != nil || gotDir != wantDir {
		t.Fatalf("local working directory: %q, %v; want %q", gotDir, err, wantDir)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := environment.RunInDir(ctx, dir, ".", nil, &stdout, &stderr, "/bin/sh", "-c", "exit 0"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
