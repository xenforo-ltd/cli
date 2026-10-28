package gitenv

import (
	"strings"
	"testing"
)

// TestEnvironmentDropsRepositoryLocation covers the isolation every Git caller
// relies on: a repository, worktree, index, or object store named by the caller
// must not replace the directory the command was given.
func TestEnvironmentDropsRepositoryLocation(t *testing.T) {
	redirecting := []string{
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_COMMON_DIR",
		"GIT_PREFIX",
	}

	for _, key := range redirecting {
		t.Setenv(key, "/elsewhere")
	}

	t.Setenv("GIT_AUTHOR_NAME", "Test")

	present := make(map[string]bool)

	for _, entry := range Environment() {
		key, _, _ := strings.Cut(entry, "=")
		present[key] = true
	}

	for _, key := range redirecting {
		if present[key] {
			t.Errorf("Environment kept %s", key)
		}
	}

	if !present["GIT_AUTHOR_NAME"] {
		t.Error("Environment dropped an unrelated variable")
	}
}
