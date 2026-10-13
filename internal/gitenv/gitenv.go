// Package gitenv builds the environment for Git commands that name their
// working directory explicitly.
package gitenv

import (
	"os"
	"strings"
)

// Environment returns the process environment without the variables that point
// Git at a particular repository, worktree, index, or object store.
//
// xf always passes the directory a Git command should run in. A GIT_DIR or
// GIT_WORK_TREE inherited from a hook, an IDE, or a nested Git invocation would
// override that directory, so the command could read or modify a different
// checkout instead.
func Environment() []string {
	redirecting := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_INDEX_FILE":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR":                   true,
		"GIT_PREFIX":                       true,
	}

	env := os.Environ()
	kept := make([]string, 0, len(env))

	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if redirecting[key] {
			continue
		}

		kept = append(kept, entry)
	}

	return kept
}
