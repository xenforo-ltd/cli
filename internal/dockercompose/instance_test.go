package dockercompose

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// xenForoDir creates a directory that satisfies NewRunner's preconditions, with
// .env written from env unless env is empty.
func xenForoDir(t *testing.T, env string, mode os.FileMode) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n"), 0o600); err != nil {
		t.Fatalf("write compose.yaml: %v", err)
	}

	if env != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), mode); err != nil {
			t.Fatalf("write .env: %v", err)
		}
	}

	return dir
}

func TestNewRunnerDefaultsToXFWithoutAnEnvFile(t *testing.T) {
	runner, err := NewRunner(xenForoDir(t, "", 0o600))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	if runner.Instance() != "xf" {
		t.Errorf("Instance = %q, want the xf default", runner.Instance())
	}
}

func TestNewRunnerReadsTheInstanceAndContexts(t *testing.T) {
	runner, err := NewRunner(xenForoDir(t, "XF_INSTANCE=feature\nXF_CONTEXTS=mysql:redis\n", 0o600))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	if runner.Instance() != "feature" {
		t.Errorf("Instance = %q, want feature", runner.Instance())
	}

	if got := runner.Contexts(); len(got) != 2 || got[0] != "mysql" || got[1] != "redis" {
		t.Errorf("Contexts = %v, want [mysql redis]", got)
	}
}

// TestNewRunnerReportsAnUnreadableEnvFile is the guard against tearing down the
// wrong project. Destroy runs "docker compose down --volumes" for the runner's
// instance, so falling back to "xf" for a .env that exists but cannot be read
// would target the source environment's project instead of the checkout's own.
func TestNewRunnerReportsAnUnreadableEnvFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unreadable-file permissions are not enforced the same way on Windows")
	}

	if os.Geteuid() == 0 {
		t.Skip("root can read a file regardless of its permissions")
	}

	dir := xenForoDir(t, "XF_INSTANCE=feature\n", 0o000)

	if _, err := NewRunner(dir); err == nil {
		t.Fatal("expected an unreadable .env to be reported rather than defaulted")
	}
}

func TestReadInstanceName(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"a missing file", "", ""},
		{"a file without the value", "XF_CONTEXTS=mysql\n", ""},
		{"a file with the value", "XF_INSTANCE=feature\n", "feature"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadInstanceName(xenForoDir(t, tc.content, 0o600))
			if err != nil {
				t.Fatalf("ReadInstanceName: %v", err)
			}

			if got != tc.want {
				t.Errorf("ReadInstanceName = %q, want %q", got, tc.want)
			}
		})
	}
}
