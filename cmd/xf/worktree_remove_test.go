package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/xenforo-ltd/cli/internal/worktree"
)

// writeEnvironment marks dir as a configured checkout using instance, writing
// the files NewRunner requires.
func writeEnvironment(t *testing.T, dir, instance string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose.yaml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("XF_INSTANCE="+instance+"\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

// TestRemoveRefusesTheSourceEnvironment covers an adopted checkout that copies
// the source's .env, so both name the same Docker project. Tearing the checkout
// down would delete the source's database and volumes, so removal must stop
// before Docker is invoked, with or without --force.
func TestRemoveRefusesTheSourceEnvironment(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			repo := newGitXenForoRepo(t)
			registry := useTempRegistry(t)

			writeEnvironment(t, repo, "xf")

			adopted := filepath.Join(t.TempDir(), "elsewhere")
			runGitIn(t, repo, "worktree", "add", "-q", adopted, "-b", "feature")
			writeEnvironment(t, adopted, "xf")

			if err := registry.Add(worktree.Entry{
				SourcePath:   repo,
				SourceBranch: "main",
				WorktreePath: adopted,
				Branch:       "feature",
				Instance:     "xf",
			}); err != nil {
				t.Fatalf("Add: %v", err)
			}

			logPath := writeFakeDocker(t)

			oldForce, oldKeep := flagWorktreeForce, flagWorktreeKeepContainers
			flagWorktreeForce, flagWorktreeKeepContainers = force, false

			t.Cleanup(func() {
				flagWorktreeForce, flagWorktreeKeepContainers = oldForce, oldKeep
			})

			t.Chdir(repo)

			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())

			err := runWorktreeRemove(cmd, []string{"feature"})
			if err == nil || !strings.Contains(err.Error(), "--keep-containers") {
				t.Fatalf("runWorktreeRemove = %v, want a refusal that names --keep-containers", err)
			}

			if entries := readDockerLog(t, logPath); len(entries) > 0 {
				t.Errorf("docker was called: %s", entries)
			}

			if _, statErr := os.Stat(adopted); statErr != nil {
				t.Errorf("the adopted checkout was changed: %v", statErr)
			}
		})
	}
}

// TestRemoveKeepsTheBranchCheckForAStaleRecord covers a registry record that
// still names the branch a conventional worktree held before someone switched
// branches in it. Resolving "feature" prefers that record over the directory
// name, so removal must compare the checkout's real owner with it. Treating the
// directory name as the selection would skip the comparison and delete the
// branch that is really checked out.
func TestRemoveKeepsTheBranchCheckForAStaleRecord(t *testing.T) {
	repo := newGitXenForoRepo(t)
	registry := useTempRegistry(t)

	conventional := filepath.Join(worktree.WorktreesDir(repo), "feature")
	runGitIn(t, repo, "worktree", "add", "-q", conventional, "-b", "feature")

	if err := registry.Add(worktree.Entry{
		SourcePath:   repo,
		SourceBranch: "main",
		WorktreePath: conventional,
		Branch:       "feature",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The record still says "feature", but the checkout no longer holds it.
	runGitIn(t, conventional, "switch", "-q", "-c", "other")

	logPath := writeFakeDocker(t)

	oldForce, oldKeep := flagWorktreeForce, flagWorktreeKeepContainers
	flagWorktreeForce, flagWorktreeKeepContainers = false, false

	t.Cleanup(func() {
		flagWorktreeForce, flagWorktreeKeepContainers = oldForce, oldKeep
	})

	t.Chdir(repo)

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	err := runWorktreeRemove(cmd, []string{"feature"})
	if err == nil || !strings.Contains(err.Error(), "other checked out, not feature") {
		t.Fatalf("runWorktreeRemove = %v, want a refusal that names the real branch", err)
	}

	if entries := readDockerLog(t, logPath); len(entries) > 0 {
		t.Errorf("docker was called: %s", entries)
	}

	if _, statErr := os.Stat(conventional); statErr != nil {
		t.Errorf("the checkout was changed: %v", statErr)
	}

	runGitIn(t, repo, "show-ref", "--verify", "--quiet", "refs/heads/other")
}

// writeFakeDocker puts a fake docker first on PATH, so an unexpected teardown
// is recorded rather than run. Both the Unix and Windows shims are written;
// Windows ignores the extensionless file and finds docker.cmd through PATHEXT.
// It returns the file the fake appends its arguments to.
func writeFakeDocker(t *testing.T) string {
	t.Helper()

	bin := t.TempDir()
	logPath := filepath.Join(bin, "docker.log")

	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DOCKER_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}

	windowsScript := "@echo off\r\n>>\"%DOCKER_LOG%\" echo %*\r\n"
	if err := os.WriteFile(filepath.Join(bin, "docker.cmd"), []byte(windowsScript), 0o700); err != nil {
		t.Fatalf("write Windows fake docker: %v", err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_LOG", logPath)

	return logPath
}

// readDockerLog returns what the fake docker was asked to run, or nothing when
// it was never invoked.
func readDockerLog(t *testing.T, path string) []byte {
	t.Helper()

	entries, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		t.Fatalf("read docker log: %v", err)
	}

	return entries
}
