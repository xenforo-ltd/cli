package environment_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/environment"
)

func runnerWithFakeDocker(t *testing.T, script string) *environment.Runner {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	dir := installation(t)
	writeFile(t, filepath.Join(dir, "compose.yaml"), "")
	bin := t.TempDir()
	path := filepath.Join(bin, "docker")
	writeFile(t, path, "#!/bin/sh\n"+script+"\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	runner, err := environment.NewRunner(dir)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestPSInfoComposeFormats(t *testing.T) {
	records := []string{
		`{"Service":"web","Name":"forum-web","State":"running","Health":"healthy","Publishers":[{"URL":"0.0.0.0","TargetPort":80,"PublishedPort":8080,"Protocol":"tcp"},{"URL":"::1","TargetPort":443,"PublishedPort":8443,"Protocol":"tcp"},{"TargetPort":9000,"Protocol":"udp"}]}`,
		`{"Service":"worker","Name":"forum-worker","State":"exited","ExitCode":7,"Publishers":null}`,
	}
	for _, tc := range []struct{ name, data string }{
		{"array", "[" + strings.Join(records, ",") + "]"},
		{"lines", strings.Join(records, "\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runnerWithFakeDocker(t, `printf '%s' "$PS_DATA"`)
			t.Setenv("PS_DATA", tc.data)
			got, err := r.PSInfo(t.Context())
			if err != nil || len(got) != 2 {
				t.Fatalf("PSInfo: %v, %v", got, err)
			}
			if got[0].Service != "web" || got[0].Name != "forum-web" || got[0].State != "running" || got[0].Status != "healthy" || got[0].Ports != "0.0.0.0:8080->80/tcp, [::1]:8443->443/tcp, 9000/udp" {
				t.Errorf("running container: %+v", got[0])
			}
			if got[1].Service != "worker" || got[1].State != "exited" || got[1].Status != "exit 7" || got[1].Ports != "—" {
				t.Errorf("exited container: %+v", got[1])
			}
		})
	}
}

func TestPSInfoErrorsAndEmpty(t *testing.T) {
	r := runnerWithFakeDocker(t, `if [ "$PS_FAIL" = 1 ]; then echo 'daemon unavailable' >&2; exit 42; fi
printf '%s' "$PS_DATA"`)
	for _, data := range []string{`[{"Service":`, "{\"Service\":\"ok\"}\n{"} {
		t.Setenv("PS_DATA", data)
		if _, err := r.PSInfo(t.Context()); err == nil || !strings.Contains(err.Error(), "failed to parse container status") {
			t.Errorf("malformed %q: %v", data, err)
		}
	}
	t.Setenv("PS_DATA", " \n")
	if got, err := r.PSInfo(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("empty output: %v, %v", got, err)
	}
	t.Setenv("PS_FAIL", "1")
	if _, err := r.PSInfo(t.Context()); err == nil || !strings.Contains(err.Error(), "daemon unavailable") {
		t.Errorf("docker failure: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.PSInfo(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancellation: %v", err)
	}
}

func TestWaitForDatabaseRetryAndFailure(t *testing.T) {
	log := filepath.Join(t.TempDir(), "attempts")
	t.Setenv("ATTEMPTS", log)
	r := runnerWithFakeDocker(t, `printf 'attempt\n' >> "$ATTEMPTS"
if [ "$READY" = yes ]; then
  if [ ! -f "$ATTEMPTS.first" ]; then : > "$ATTEMPTS.first"; exit 1; fi
  printf OK
fi`)
	t.Setenv("READY", "yes")
	if err := r.WaitForDatabase(t.Context(), 0); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := strings.Count(readFile(t, log), "attempt\n"); got != 2 {
		t.Fatalf("attempts before success: %d", got)
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	t.Setenv("READY", "no")
	if err := r.WaitForDatabase(t.Context(), 0); err == nil || !strings.Contains(err.Error(), "timed out waiting for database") {
		t.Fatalf("not ready: %v", err)
	}
	if got := strings.Count(readFile(t, log), "attempt\n"); got <= 1 {
		t.Errorf("expected retries before failure, got %d attempts", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.WaitForDatabase(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("cancellation: %v", err)
	}
}

func TestDatabaseCredentialResolution(t *testing.T) {
	dir := installation(t)
	writeFile(t, filepath.Join(dir, "compose.yaml"), "")
	writeFile(t, filepath.Join(dir, ".env"), "XF_DB_USER=file-user\nXF_DB_PASSWORD=file-password\nXF_DB_DATABASE=file-db\n")
	r, err := environment.NewRunner(dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(user, password, name string) {
		t.Helper()
		gotUser, gotPassword := r.DatabaseCredentials()
		if gotUser != user || gotPassword != password || r.DatabaseName() != name {
			t.Errorf("got user=%q password=%q database=%q; want %q %q %q", gotUser, gotPassword, r.DatabaseName(), user, password, name)
		}
	}
	t.Setenv("XF_DB_USER", "process-user")
	t.Setenv("XF_DB_PASSWORD", "process-password")
	t.Setenv("XF_DB_DATABASE", "process-db")
	check("process-user", "process-password", "process-db")
	t.Setenv("XF_DB_USER", "")
	t.Setenv("XF_DB_PASSWORD", "")
	t.Setenv("XF_DB_DATABASE", "")
	check("file-user", "file-password", "file-db")
	if err := os.Remove(filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	check("xf", "password", "xf")
}
