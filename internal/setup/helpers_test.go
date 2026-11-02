package setup_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/setup"
)

func TestInstallInvocationQuotesArgumentsAndKeepsPasswordOutOfShellCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tool fixture requires a POSIX shell")
	}
	dir := t.TempDir()
	php := filepath.Join(dir, "php")
	if err := os.WriteFile(php, []byte("#!/bin/sh\nprintf '%s\\0' \"$@\" > \"$OUTPUT\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	user, email := "user' $(touch injected)", "a b@example.com"
	title, url, password := "Title'; touch injected; #", "https://example.test/a?x=1&y=2", "secret $* ' \n"
	args, env := setup.InstallInvocation(user, email, title, url, password)
	if len(args) != 3 || args[0] != "sh" || args[1] != "-c" || strings.Contains(strings.Join(args, " "), password) {
		t.Fatalf("unsafe shell arguments: %q", args)
	}
	if !reflect.DeepEqual(env, map[string]string{"XF_INSTALL_PASSWORD": password}) {
		t.Fatalf("environment: %v", env)
	}
	output := filepath.Join(dir, "args")
	cmd := exec.CommandContext(t.Context(), "/bin/sh", args[1:]...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + dir, "OUTPUT=" + output, "XF_INSTALL_PASSWORD=" + password}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell: %v %s", err, out)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	want := []string{"cmd.php", "xf:install", "--no-interaction", "--clear", "--user=" + user, "--email=" + email, "--title=" + title, "--url=" + url, "--password=" + password}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments: %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatalf("shell injection created a file: %v", err)
	}
}
