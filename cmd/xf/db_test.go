package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/database"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
)

// newRunnerForDB builds a Runner from a minimal XenForo directory whose .env
// contains the given contents.
func newRunnerForDB(t *testing.T, env string) *dockercompose.Runner {
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

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	runner, err := dockercompose.NewRunner(dir)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	return runner
}

func TestResolveDatabaseInfoMySQL(t *testing.T) {
	runner := newRunnerForDB(t, "XF_INSTANCE=main\nXF_CONTEXTS=caddy:mysql:development\nXF_DB_USER=xf\nXF_DB_PASSWORD=secret\nXF_DB_DATABASE=xf\n")

	info, err := resolveDatabaseInfo(runner)
	if err != nil {
		t.Fatalf("resolveDatabaseInfo returned error: %v", err)
	}

	if info.Driver != database.DriverMySQL {
		t.Errorf("Driver = %q, want %q", info.Driver, database.DriverMySQL)
	}

	if info.Host != "mysql.main.orb.local" {
		t.Errorf("Host = %q, want %q", info.Host, "mysql.main.orb.local")
	}

	if info.Port != 3306 {
		t.Errorf("Port = %d, want 3306", info.Port)
	}

	if info.User != "xf" || info.Password != "secret" || info.Name != "xf" {
		t.Errorf("credentials = %q/%q/%q, want xf/secret/xf", info.User, info.Password, info.Name)
	}

	wantURL := "mysql://xf:secret@mysql.main.orb.local:3306/xf"
	if got := info.URL(); got != wantURL {
		t.Errorf("URL() = %q, want %q", got, wantURL)
	}
}

func TestResolveDatabaseInfoRejectsUnsupportedDrivers(t *testing.T) {
	runner := newRunnerForDB(t, "XF_CONTEXTS=caddy:postgres\n")

	if _, err := resolveDatabaseInfo(runner); err == nil {
		t.Fatal("expected an error for a postgres environment")
	}
}

func TestResolveDatabaseInfoRequiresDatabaseContext(t *testing.T) {
	runner := newRunnerForDB(t, "XF_CONTEXTS=caddy:redis\n")

	if _, err := resolveDatabaseInfo(runner); err == nil {
		t.Fatal("expected an error when no database is configured")
	}
}

// TestDBCmdAcceptsOnlyOnePath matches the [path] argument the help advertises.
func TestDBCmdAcceptsOnlyOnePath(t *testing.T) {
	cmd := findCommand(t, "db")

	if cmd.Args == nil {
		t.Fatal("db command has no Args validator")
	}

	if err := cmd.Args(cmd, []string{"one", "two"}); err == nil {
		t.Fatal("expected an error for more than one path")
	}
}

// TestDBCmdRejectsMutuallyExclusiveFlags covers --print-url and --shell, which
// select different actions and must not be combined.
func TestDBCmdRejectsMutuallyExclusiveFlags(t *testing.T) {
	t.Cleanup(func() {
		flagDBPrintURL = false
		flagDBShell = false
	})

	err := runTree(t, "db", "--print-url", "--shell")
	if err == nil {
		t.Fatal("expected an error when both --print-url and --shell are set")
	}

	if _, ok := errors.AsType[*usageError](err); !ok {
		t.Fatalf("expected a usage error so usage is printed, got %T: %v", err, err)
	}
}
