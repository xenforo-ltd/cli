package dockercompose

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const windowsOS = "windows"

var (
	sharedFakeDockerPath string
	errSharedFakeDocker  error
	sharedFakeDockerOnce sync.Once
)

func TestBuildComposeArgsIncludesContextAndOverride(t *testing.T) {
	tmp := t.TempDir()

	files := []string{
		"compose.yaml",
		"compose.mysql.yaml",
		"compose.override.yaml",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte("services: {}\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	runner := &Runner{
		xfDir:    tmp,
		instance: "demo",
		contexts: []string{"mysql", "redis"},
	}

	got := runner.buildComposeArgs()
	want := []string{
		"compose",
		"--project-name", "demo",
		"--file", filepath.Join(tmp, "compose.yaml"),
		"--file", filepath.Join(tmp, "compose.mysql.yaml"),
		"--file", filepath.Join(tmp, "compose.override.yaml"),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildComposeArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestIsServiceRunning(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("fake docker shim test is unix-only")
	}

	t.Run("running", func(t *testing.T) {
		runner, _ := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "running")

		running, err := runner.isServiceRunning(t.Context(), "xf")
		if err != nil {
			t.Fatalf("isServiceRunning returned error: %v", err)
		}

		if !running {
			t.Fatal("expected service to be running")
		}
	})

	t.Run("stopped", func(t *testing.T) {
		runner, _ := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "stopped")

		running, err := runner.isServiceRunning(t.Context(), "xf")
		if err != nil {
			t.Fatalf("isServiceRunning returned error: %v", err)
		}

		if running {
			t.Fatal("expected service to be stopped")
		}
	})

	t.Run("error", func(t *testing.T) {
		runner, _ := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "error")

		if _, err := runner.isServiceRunning(t.Context(), "xf"); err == nil {
			t.Fatal("expected error when docker ps probe fails")
		}
	})
}

func TestExecOrRunBranching(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("fake docker shim test is unix-only")
	}

	t.Run("running uses exec", func(t *testing.T) {
		runner, logFile := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "running")
		t.Setenv("DOCKER_EXEC_MODE", "ok")

		if err := runner.ExecOrRun(t.Context(), "xf", true, "php", "-v"); err != nil {
			t.Fatalf("ExecOrRun returned error: %v", err)
		}

		log := readDockerLog(t, logFile)
		if !strings.Contains(log, " exec xf php -v") {
			t.Fatalf("expected exec invocation, log:\n%s", log)
		}

		if strings.Contains(log, " run --rm xf php -v") {
			t.Fatalf("did not expect run invocation, log:\n%s", log)
		}
	})

	t.Run("stopped uses run", func(t *testing.T) {
		runner, logFile := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "stopped")

		if err := runner.ExecOrRun(t.Context(), "xf", true, "php", "-v"); err != nil {
			t.Fatalf("ExecOrRun returned error: %v", err)
		}

		log := readDockerLog(t, logFile)
		if !strings.Contains(log, " run --rm xf php -v") {
			t.Fatalf("expected run invocation, log:\n%s", log)
		}

		if strings.Contains(log, " exec xf php -v") {
			t.Fatalf("did not expect exec invocation, log:\n%s", log)
		}
	})

	t.Run("exec not-running error falls back to run", func(t *testing.T) {
		runner, logFile := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "running")
		t.Setenv("DOCKER_EXEC_MODE", "not_running")

		if err := runner.ExecOrRun(t.Context(), "xf", true, "php", "-v"); err != nil {
			t.Fatalf("ExecOrRun returned error: %v", err)
		}

		log := readDockerLog(t, logFile)
		if !strings.Contains(log, " exec xf php -v") || !strings.Contains(log, " run --rm xf php -v") {
			t.Fatalf("expected exec then run fallback, log:\n%s", log)
		}
	})
}

func TestExecOrRunWithEnvBranching(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("fake docker shim test is unix-only")
	}

	t.Run("running uses exec with env", func(t *testing.T) {
		runner, logFile := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "running")
		t.Setenv("DOCKER_EXEC_MODE", "ok")

		if err := runner.ExecOrRunWithEnv(t.Context(), "xf", true, map[string]string{"XDEBUG_SESSION": "1"}, "php", "-v"); err != nil {
			t.Fatalf("ExecOrRunWithEnv returned error: %v", err)
		}

		log := readDockerLog(t, logFile)
		if !strings.Contains(log, " exec -e XDEBUG_SESSION=1 xf php -v") {
			t.Fatalf("expected exec invocation with env, log:\n%s", log)
		}

		if strings.Contains(log, " run --rm --env XDEBUG_SESSION=1 xf php -v") {
			t.Fatalf("did not expect run invocation, log:\n%s", log)
		}
	})

	t.Run("stopped uses run with env", func(t *testing.T) {
		runner, logFile := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PS_MODE", "stopped")

		if err := runner.ExecOrRunWithEnv(t.Context(), "xf", true, map[string]string{"XDEBUG_SESSION": "1"}, "php", "-v"); err != nil {
			t.Fatalf("ExecOrRunWithEnv returned error: %v", err)
		}

		log := readDockerLog(t, logFile)
		if !strings.Contains(log, " run --rm --env XDEBUG_SESSION=1 xf php -v") {
			t.Fatalf("expected run invocation with env, log:\n%s", log)
		}

		if strings.Contains(log, " exec -e XDEBUG_SESSION=1 xf php -v") {
			t.Fatalf("did not expect exec invocation, log:\n%s", log)
		}
	})
}

func newRunnerWithFakeDocker(t *testing.T) (*Runner, string) {
	t.Helper()

	xfDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(xfDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose.yaml: %v", err)
	}

	sharedFakeDockerOnce.Do(func() {
		binDir, err := os.MkdirTemp("", "xf-fake-docker-*")
		if err != nil {
			errSharedFakeDocker = fmt.Errorf("create fake docker dir: %w", err)
			return
		}

		sharedFakeDockerPath = filepath.Join(binDir, "docker")

		script := `#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${DOCKER_LOG_FILE:-}" ]]; then
  printf '%s\n' "$*" >> "$DOCKER_LOG_FILE"
fi
args=" $* "
if [[ "$args" == *" ps --status running --services "* ]]; then
  mode="${DOCKER_PS_MODE:-running}"
  if [[ "$mode" == "running" ]]; then
    echo "xf"
    exit 0
  fi
  if [[ "$mode" == "stopped" ]]; then
    exit 0
  fi
  echo "ps failed" >&2
  exit 1
fi
if [[ "$args" == *" exec "* ]]; then
  mode="${DOCKER_EXEC_MODE:-ok}"
  if [[ "$mode" == "not_running" ]]; then
    echo 'service "xf" is not running' >&2
    exit 1
  fi
  if [[ "$mode" == "fail" ]]; then
    echo "exec failed" >&2
    exit 1
  fi
  exit 0
fi
if [[ "$args" == *" run "* ]]; then
  exit 0
fi
exit 0
`
		if err := os.WriteFile(sharedFakeDockerPath, []byte(script), 0o700); err != nil {
			errSharedFakeDocker = fmt.Errorf("write fake docker: %w", err)
		}
	})

	if errSharedFakeDocker != nil {
		t.Fatalf("set up fake docker: %v", errSharedFakeDocker)
	}

	logFile := filepath.Join(t.TempDir(), "docker.log")
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", filepath.Dir(sharedFakeDockerPath), os.PathListSeparator, os.Getenv("PATH")))
	t.Setenv("DOCKER_LOG_FILE", logFile)

	runner := &Runner{
		xfDir:    xfDir,
		instance: "demo",
	}

	return runner, logFile
}

func readDockerLog(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}

	return string(data)
}
