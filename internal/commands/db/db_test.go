package db_test

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/commands/db"
)

func databaseFixture(t *testing.T, contexts string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"src/XF.php": "<?php", "compose.yaml": "",
		".env": "XF_INSTANCE=forum\nXF_CONTEXTS=" + contexts + "\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "docker.log")
	script := `#!/bin/sh
printf '%s\n' 'CALL' "$@" >> "$DOCKER_LOG"
case " $* " in
  *' info '*) if [ "$INFO_FAIL" = 1 ]; then exit 1; fi; printf '%s\n' "$ENGINE" ;;
  *' ps '*) if [ "$RUNNING" = yes ]; then printf 'mysql\n'; fi ;;
  *' exec '*) printf 'PASSWORD=%s\n' "$MYSQL_PWD" >> "$DOCKER_LOG" ;;
esac
`
	path := filepath.Join(bin, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("DOCKER_LOG", log)
	t.Setenv("ENGINE", "OrbStack")
	t.Setenv("RUNNING", "yes")
	t.Setenv("XF_DB_USER", "user@name")
	t.Setenv("XF_DB_PASSWORD", "p@ss:/ word")
	t.Setenv("XF_DB_DATABASE", "forum db")
	return dir, log
}

func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	callErr := fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output), callErr
}

func TestPrintURLAndOrbStackRequirement(t *testing.T) {
	dir, _ := databaseFixture(t, "redis:mysql")
	output, err := captureOutput(t, func() error { return db.DB(t.Context(), []string{dir}, db.Options{PrintURL: true}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "user%40name:p%40ss%3A%2F%20word@") || !strings.Contains(output, "forum%20db") {
		t.Fatalf("credentials or database not escaped: %q", output)
	}
	u, err := url.Parse(strings.TrimSpace(output))
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if u.Scheme != "mysql" || u.Host != "mysql.forum.orb.local:3306" || u.User.Username() != "user@name" || password != "p@ss:/ word" || u.Path != "/forum db" {
		t.Fatalf("unexpected connection URL: %s", u)
	}
	t.Setenv("ENGINE", "Docker Desktop")
	if _, err := captureOutput(t, func() error { return db.DB(t.Context(), []string{dir}, db.Options{PrintURL: true}) }); err == nil || !strings.Contains(err.Error(), "requires OrbStack") {
		t.Errorf("non-OrbStack engine: %v", err)
	}
	t.Setenv("INFO_FAIL", "1")
	if _, err := captureOutput(t, func() error { return db.DB(t.Context(), []string{dir}, db.Options{PrintURL: true}) }); err == nil || !strings.Contains(err.Error(), "failed to detect OrbStack") {
		t.Errorf("docker info failure: %v", err)
	}
}

func TestDatabaseContextSelection(t *testing.T) {
	for _, tc := range []struct{ contexts, want string }{
		{"redis", "no supported database context"},
		{"postgres", "postgres databases are not supported"},
		{"mssql", "mssql databases are not supported"},
		{"sqlite", "sqlite databases are not supported"},
	} {
		t.Run(tc.contexts, func(t *testing.T) {
			dir, _ := databaseFixture(t, tc.contexts)
			for _, run := range []func() error{
				func() error { return db.DB(t.Context(), []string{dir}, db.Options{PrintURL: true}) },
				func() error { return db.Shell(t.Context(), []string{dir}) },
			} {
				if err := run(); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("got %v, want %q", err, tc.want)
				}
			}
		})
	}
}

func TestShellForwardsArgumentsWithoutPasswordInArgv(t *testing.T) {
	dir, log := databaseFixture(t, "mysql-replication")
	if err := db.Shell(t.Context(), []string{dir, "--", "--execute", "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, part := range []string{"\nmariadb\n", "\n--user=user@name\n", "\n--database=forum db\n", "\n--execute\nSELECT 1\n", "PASSWORD=p@ss:/ word\n"} {
		if !strings.Contains(got, part) {
			t.Errorf("missing %q in Docker log %q", part, got)
		}
	}
	if strings.Count(got, "p@ss:/ word") != 1 {
		t.Errorf("password appeared in arguments: %q", got)
	}
	t.Setenv("RUNNING", "no")
	if err := db.Shell(t.Context(), []string{dir}); err == nil || !strings.Contains(err.Error(), "database is not running") {
		t.Errorf("stopped database: %v", err)
	}
}
