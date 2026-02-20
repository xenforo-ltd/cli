package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clierrors "xf/internal/errors"
	"xf/internal/xf"
)

func TestFindXenForoDirFindsParent(t *testing.T) {
	t.Setenv("XF_DIR", "")

	root := t.TempDir()
	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(xfFile, []byte("<?php"), 0644); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	detected, err := xf.GetXenForoDir(nested)
	if err != nil {
		t.Fatalf("GetXenForoDir returned error: %v", err)
	}
	if detected != root {
		t.Fatalf("detected dir = %q, want %q", detected, root)
	}
}

func TestFindXenForoDirTerminatesAtRoot(t *testing.T) {
	t.Setenv("XF_DIR", "")

	done := make(chan struct{})
	go func() {
		_, _ = xf.GetXenForoDir(string(filepath.Separator))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetXenForoDir did not terminate")
	}
}

func TestRunAsXenForoCommandOutsideDirReturnsActionableError(t *testing.T) {
	t.Setenv("XF_DIR", "")
	setCwd(t, t.TempDir())

	err := runAsXenForoCommand([]string{"list"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !clierrors.Is(err, clierrors.CodeInvalidInput) {
		t.Fatalf("expected invalid input code, got: %v", err)
	}
	if got := err.Error(); got == "" || !containsAll(got, "unknown command", "not in a XenForo directory") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestIsKnownCommandIncludesCobraCompletion(t *testing.T) {
	if !isKnownCommand("completion") {
		t.Fatal("expected completion to be treated as a known command")
	}
}

func TestRunAsXenForoCommandFallsBackToLocalWhenComposeMissing(t *testing.T) {
	t.Setenv("XF_DIR", "")

	root := t.TempDir()
	xfFile := filepath.Join(root, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfFile), 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(xfFile, []byte("<?php"), 0644); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	setCwd(t, root)

	execCommand = helperCommand(t,
		"php cmd.php xf-dev:import",
		root,
		0,
	)
	t.Cleanup(func() {
		execCommand = exec.Command
	})

	if err := runAsXenForoCommand([]string{"xf-dev:import"}); err != nil {
		t.Fatalf("runAsXenForoCommand returned error: %v", err)
	}
}

func TestRunAsLocalXenForoCommandBuildsExpectedInvocation(t *testing.T) {
	root := t.TempDir()
	execCommand = helperCommand(t,
		"php cmd.php cron:run --verbose",
		root,
		0,
	)
	t.Cleanup(func() {
		execCommand = exec.Command
	})

	if err := runAsLocalXenForoCommand(root, []string{"cron:run", "--verbose"}); err != nil {
		t.Fatalf("runAsLocalXenForoCommand returned error: %v", err)
	}
}

func TestRunAsLocalXenForoCommandReturnsActionableErrorWhenPHPMissing(t *testing.T) {
	root := t.TempDir()
	execCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("__xf_missing_php_binary__")
	}
	t.Cleanup(func() {
		execCommand = exec.Command
	})

	err := runAsLocalXenForoCommand(root, []string{"list"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !clierrors.Is(err, clierrors.CodeInvalidInput) {
		t.Fatalf("expected invalid input code, got: %v", err)
	}
	if got := err.Error(); !containsAll(got, "PHP", "PATH") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestRunAsLocalXenForoCommandReturnsErrorOnNonZeroExit(t *testing.T) {
	root := t.TempDir()
	execCommand = helperCommand(t,
		"php cmd.php list",
		root,
		2,
	)
	t.Cleanup(func() {
		execCommand = exec.Command
	})

	err := runAsLocalXenForoCommand(root, []string{"list"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsAll(err.Error(), "local XenForo command failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func helperCommand(t *testing.T, expectedArgs, expectedWd string, exitCode int) func(string, ...string) *exec.Cmd {
	t.Helper()
	expectedWd = canonicalPath(t, expectedWd)

	return func(command string, args ...string) *exec.Cmd {
		cs := make([]string, 0, len(args)+3)
		cs = append(cs, "-test.run=TestHelperProcess", "--", command)
		cs = append(cs, args...)

		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_EXPECT_ARGS="+expectedArgs,
			"HELPER_EXPECT_WD="+expectedWd,
			fmt.Sprintf("HELPER_EXIT_CODE=%d", exitCode),
		)
		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	dash := -1
	for i, arg := range os.Args {
		if arg == "--" {
			dash = i
			break
		}
	}
	if dash == -1 {
		fmt.Fprintln(os.Stderr, "missing -- separator")
		os.Exit(2)
	}

	gotArgs := strings.Join(os.Args[dash+1:], " ")
	if wantArgs := os.Getenv("HELPER_EXPECT_ARGS"); wantArgs != "" && gotArgs != wantArgs {
		fmt.Fprintf(os.Stderr, "args mismatch: got %q want %q\n", gotArgs, wantArgs)
		os.Exit(3)
	}

	if wantWd := os.Getenv("HELPER_EXPECT_WD"); wantWd != "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "getwd failed: %v\n", err)
			os.Exit(4)
		}
		if resolved, err := filepath.EvalSymlinks(wd); err == nil {
			wd = filepath.Clean(resolved)
		} else {
			wd = filepath.Clean(wd)
		}
		if wd != filepath.Clean(wantWd) {
			fmt.Fprintf(os.Stderr, "cwd mismatch: got %q want %q\n", wd, filepath.Clean(wantWd))
			os.Exit(5)
		}
	}

	code := 0
	if _, err := fmt.Sscanf(os.Getenv("HELPER_EXIT_CODE"), "%d", &code); err != nil {
		fmt.Fprintf(os.Stderr, "invalid HELPER_EXIT_CODE: %v\n", err)
		os.Exit(6)
	}
	os.Exit(code)
}
