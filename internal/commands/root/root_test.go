package root_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
	rootcommand "github.com/xenforo-ltd/cli/internal/commands/root"
)

func localInstallation(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tool fixture requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("XF_DIR", "")
	t.Chdir(filepath.Join(dir, "src"))
	return dir, bin
}

func TestLocalCommandPreservesArgumentsAndChildStatus(t *testing.T) {
	dir, bin := localInstallation(t)
	log := filepath.Join(t.TempDir(), "invocation")
	t.Setenv("INVOCATION", log)
	t.Setenv("CHILD_EXIT", "0")
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\" > \"$INVOCATION\"\nexit \"${CHILD_EXIT:-0}\"\n"
	if err := os.WriteFile(filepath.Join(bin, "php"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	args := []string{"xf-dev:import", "--verbose", "value with spaces", "--", "-x"}
	if err := rootcommand.Run(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no invocation recorded")
	}
	gotDir, err := filepath.EvalSymlinks(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != wantDir || !reflect.DeepEqual(lines[1:], append([]string{"cmd.php"}, args...)) {
		t.Fatalf("invocation = %q, want installation directory followed by cmd.php and %q", lines, args)
	}
	t.Setenv("CHILD_EXIT", "37")
	err = rootcommand.Run(t.Context(), args)
	if code, ok := clierror.ExitCode(err); !ok || code != 37 {
		t.Fatalf("child failure = %v, status = (%d, %t)", err, code, ok)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := rootcommand.Run(ctx, args); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled invocation = %v", err)
	}
}

func TestMissingPHPHasActionableRuntimeError(t *testing.T) {
	localInstallation(t)
	err := rootcommand.Run(t.Context(), []string{"list"})
	if !errors.Is(err, exec.ErrNotFound) || clierror.HintOf(err) == "" || clierror.IsUsage(err) {
		t.Fatalf("missing PHP must retain its cause and remediation without requesting usage: %v", err)
	}
}

func TestUnknownCommandOutsideInstallationHasHelpHint(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XF_DIR", "")
	err := rootcommand.Run(t.Context(), []string{"xf-dev:import"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") || !strings.Contains(clierror.HintOf(err), "xf --help") {
		t.Fatalf("unknown command error = %v, hint = %q", err, clierror.HintOf(err))
	}
}
