package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/cli"
)

// run executes the CLI in dir and returns the exit status and captured output.
func run(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(dir)
	t.Setenv("XF_DIR", "")
	var stdout, stderr bytes.Buffer
	code := cli.Run(t.Context(), args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunWritesHelpToStdout(t *testing.T) {
	code, stdout, stderr := run(t, t.TempDir(), "--help")
	if code != 0 {
		t.Fatalf("exit status = %d, want 0; stderr: %s", code, stderr)
	}
	for _, want := range []string{"Usage:", "Getting started:", "Environment:", "Run tools:", "Maintenance:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q", want)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRunPrintsHelpWhenSubcommandOmitted(t *testing.T) {
	code, stdout, stderr := run(t, t.TempDir(), "auth")
	if code != 0 {
		t.Fatalf("exit status = %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Manage OAuth authentication") {
		t.Errorf("stdout missing the command help: %q", stdout)
	}
}

func TestRunRejectsUnknownSubcommandWithUsage(t *testing.T) {
	code, stdout, stderr := run(t, t.TempDir(), "auth", "bogus")
	if code != 1 {
		t.Fatalf("exit status = %d, want 1", code)
	}
	if !strings.Contains(stderr, `unknown command "bogus"`) {
		t.Errorf("stderr missing the error: %q", stderr)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr missing usage for an invocation mistake: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRunRejectsUnknownFlagWithUsage(t *testing.T) {
	code, _, stderr := run(t, t.TempDir(), "auth", "--bogus")
	if code != 1 {
		t.Fatalf("exit status = %d, want 1", code)
	}
	if !strings.Contains(stderr, "unknown flag: --bogus") || !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr = %q, want the flag error and usage", stderr)
	}
}

func TestRunReportsGlobalFlagBeforeXenForoCommand(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, dir, "--config", config, "-v", "xf-dev:import")
	if code != 1 {
		t.Fatalf("exit status = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Global flags cannot precede a XenForo command") {
		t.Errorf("stderr missing the hint: %q", stderr)
	}
}

func TestRunForwardsUnknownCommandToXenForo(t *testing.T) {
	code, stdout, stderr := run(t, t.TempDir(), "xf-dev:import")
	if code != 1 {
		t.Fatalf("exit status = %d, want 1", code)
	}
	if !strings.Contains(stderr, "not in a XenForo directory") {
		t.Errorf("stderr = %q, want the forwarding error", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRunReportsUnreadableConfigWithoutUsage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	code, _, stderr := run(t, t.TempDir(), "--config", missing, "-v", "bogus")
	if code != 1 {
		t.Fatalf("exit status = %d, want 1", code)
	}
	if !strings.Contains(stderr, "failed to read config file") {
		t.Errorf("stderr = %q, want the config error", stderr)
	}
	if strings.Contains(stderr, "Usage:") {
		t.Errorf("runtime failure printed usage: %q", stderr)
	}
}
