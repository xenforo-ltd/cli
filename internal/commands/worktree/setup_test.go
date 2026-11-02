package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/commands/worktree"
	"github.com/xenforo-ltd/cli/internal/environment"
)

func TestCreateWithSetupConfiguresEnvironmentWithoutContainers(t *testing.T) {
	repo, _ := fixture(t)
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "docker.log")
	t.Setenv("DOCKER_LOG", log)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$DOCKER_LOG"
exit 0
`
	if err := os.WriteFile(docker, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return worktree.CreateWorktree(t.Context(), []string{"feature"}, worktree.CreateOptions{Fresh: true, NoUp: true})
	})
	if err != nil {
		t.Fatalf("create with setup: %v; stdout: %s", err, out)
	}

	pathOut, err := capture(t, func() error { return worktree.Path(t.Context(), "feature") })
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(pathOut)
	if !strings.HasPrefix(path, repo+".worktrees"+string(filepath.Separator)) {
		t.Fatalf("worktree path = %q, want a child of %q", path, repo+".worktrees")
	}
	for _, name := range []string{"compose.yaml", ".env"} {
		if _, err := os.Stat(filepath.Join(path, name)); err != nil {
			t.Errorf("setup did not write %s: %v", name, err)
		}
	}
	values, err := environment.ReadEnvFile(filepath.Join(path, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if values["XF_INSTANCE"] != "feature" || values["XF_DEBUG"] != "1" || !strings.Contains(values["XF_TITLE"], "feature") {
		t.Errorf("unexpected environment: XF_INSTANCE=%q XF_TITLE=%q XF_DEBUG=%q", values["XF_INSTANCE"], values["XF_TITLE"], values["XF_DEBUG"])
	}
	if entries := list(t); entries[path].State != "ok" {
		t.Errorf("created worktree not registered as usable: %+v", entries[path])
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		for _, action := range []string{" up ", " run ", " exec "} {
			if strings.Contains(" "+line+" ", action) {
				t.Errorf("setup with --no-up started containers: %q", line)
			}
		}
	}
}
