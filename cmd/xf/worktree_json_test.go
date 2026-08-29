package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/worktree"
)

// The JSON output reports whether the source environment was cloned, so
// automation can tell an installed worktree from an empty one.
func TestWorktreeOutputReportsTheCloneResult(t *testing.T) {
	entry := worktree.Entry{
		SourcePath:   "/src",
		SourceBranch: "main",
		WorktreePath: "/src.worktrees/feature",
		Branch:       "dev/feature",
		Instance:     "feature",
		CreatedAt:    time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
	}

	// cloneEnvironment succeeding is what sets this, immediately before the
	// output is built.
	entry.Cloned = true

	data, err := json.Marshal(worktreeOutput{
		Path:         entry.WorktreePath,
		Branch:       entry.Branch,
		SourcePath:   entry.SourcePath,
		SourceBranch: entry.SourceBranch,
		Instance:     entry.Instance,
		Cloned:       entry.Cloned,
		CreatedAt:    entry.CreatedAt,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["cloned"] != true {
		t.Errorf("cloned = %v, want true after a successful clone", decoded["cloned"])
	}
}

func TestWorktreeOutputReportsAnUnclonedWorktree(t *testing.T) {
	data, err := json.Marshal(worktreeOutput{
		Path:   "/src.worktrees/feature",
		Branch: "dev/feature",
		Cloned: false,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The key must always be present, so consumers can branch on it without
	// checking for existence first.
	value, ok := decoded["cloned"]
	if !ok {
		t.Fatal("cloned key is missing")
	}

	if value != false {
		t.Errorf("cloned = %v, want false", value)
	}
}
