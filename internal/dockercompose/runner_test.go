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

// TestProjectExists covers the Compose project discovery used to refuse a
// second environment that would reuse another one's containers or volumes. The
// probes must key on the Compose project label: any other filter could report a
// taken name as free.
func TestProjectExists(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("fake docker shim test is unix-only")
	}

	const instance = "demo"
	const projectFilter = "com.docker.compose.project=" + instance

	t.Run("matching container", func(t *testing.T) {
		_, logFile := newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PROJECT_PS", "match")

		exists, err := ProjectExists(t.Context(), instance)
		if err != nil {
			t.Fatalf("ProjectExists returned error: %v", err)
		}

		if !exists {
			t.Fatal("expected a container with the project label to count as existing")
		}

		assertProjectProbe(t, logFile, "ps --all", projectFilter)
	})

	t.Run("volume without container", func(t *testing.T) {
		_, logFile := newRunnerWithFakeDocker(t)
		// A stopped environment keeps its volumes but has no containers, so
		// the project must still be reported as existing.
		t.Setenv("DOCKER_PROJECT_VOLUME", "match")

		exists, err := ProjectExists(t.Context(), instance)
		if err != nil {
			t.Fatalf("ProjectExists returned error: %v", err)
		}

		if !exists {
			t.Fatal("expected a volume with the project label to count as existing")
		}

		assertProjectProbe(t, logFile, "volume ls", projectFilter)
	})

	t.Run("neither resource", func(t *testing.T) {
		_, logFile := newRunnerWithFakeDocker(t)

		exists, err := ProjectExists(t.Context(), instance)
		if err != nil {
			t.Fatalf("ProjectExists returned error: %v", err)
		}

		if exists {
			t.Fatal("expected no project to be reported")
		}

		// Both probes must run: a stopped project leaves no container, so
		// stopping at the first empty result would miss its volumes.
		log := readDockerLog(t, logFile)
		if !strings.Contains(log, "ps --all") || !strings.Contains(log, "volume ls") {
			t.Fatalf("expected both probes to run, log:\n%s", log)
		}
	})

	t.Run("volume probe failure", func(t *testing.T) {
		newRunnerWithFakeDocker(t)
		t.Setenv("DOCKER_PROJECT_VOLUME", "error")

		if _, err := ProjectExists(t.Context(), instance); err == nil {
			t.Fatal("expected a failing probe to be returned as an error, not treated as absence")
		}
	})
}

// assertProjectProbe checks that a probe ran with the exact Compose project
// filter, the key that links a container or volume to its project.
func assertProjectProbe(t *testing.T, logPath, subcommand, filter string) {
	t.Helper()

	log := readDockerLog(t, logPath)

	want := subcommand + " --filter label=" + filter
	if !strings.Contains(log, want) {
		t.Fatalf("probe %q did not use the filter %q, log:\n%s", subcommand, filter, log)
	}
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

		// The name alone goes in the arguments; the value is forwarded through
		// the environment so it never appears in the host's process list.
		if !strings.Contains(log, " exec -e XDEBUG_SESSION xf php -v") {
			t.Fatalf("expected exec invocation with env, log:\n%s", log)
		}

		if strings.Contains(log, "XDEBUG_SESSION=1 xf php -v") {
			t.Fatalf("env value leaked into the docker arguments, log:\n%s", log)
		}

		if !strings.Contains(log, "env XDEBUG_SESSION=1") {
			t.Fatalf("env value did not reach the docker process, log:\n%s", log)
		}

		if strings.Contains(log, " run --rm --env XDEBUG_SESSION xf php -v") {
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
		if !strings.Contains(log, " run --rm --env XDEBUG_SESSION xf php -v") {
			t.Fatalf("expected run invocation with env, log:\n%s", log)
		}

		if strings.Contains(log, "XDEBUG_SESSION=1 xf php -v") {
			t.Fatalf("env value leaked into the docker arguments, log:\n%s", log)
		}

		if !strings.Contains(log, "env XDEBUG_SESSION=1") {
			t.Fatalf("env value did not reach the docker process, log:\n%s", log)
		}

		if strings.Contains(log, " exec -e XDEBUG_SESSION xf php -v") {
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
  # Record the forwarded value so tests can prove it arrives through the
  # environment rather than the argument list.
  printf 'env XDEBUG_SESSION=%s\n' "${XDEBUG_SESSION:-<unset>}" >> "$DOCKER_LOG_FILE"
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
if [[ "$args" == *" ps --all "* ]]; then
  mode="${DOCKER_PROJECT_PS:-none}"
  if [[ "$mode" == "match" ]]; then
    echo "container-id"
    exit 0
  fi
  if [[ "$mode" == "error" ]]; then
    echo "ps --all failed" >&2
    exit 1
  fi
  exit 0
fi
if [[ "$args" == *" volume ls "* ]]; then
  mode="${DOCKER_PROJECT_VOLUME:-none}"
  if [[ "$mode" == "match" ]]; then
    echo "demo_data"
    exit 0
  fi
  if [[ "$mode" == "error" ]]; then
    echo "volume ls failed" >&2
    exit 1
  fi
  exit 0
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
