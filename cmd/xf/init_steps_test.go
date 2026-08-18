package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The step total and the Composer gate read the same decision, so the printed
// "step N of M" sequence always ends at M.
func TestComposerDecisionDrivesTheStepTotal(t *testing.T) {
	cases := []struct {
		name          string
		composerJSON  bool
		skipComposer  bool
		wantComposer  bool
		wantTotalStep int
	}{
		{"repository checkout", true, false, true, 8},
		{"release package", false, false, false, 7},
		{"checkout with --skip-composer", true, true, false, 7},
		{"release with --skip-composer", false, true, false, 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			if tc.composerJSON {
				path := filepath.Join(dir, "composer.json")
				if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
					t.Fatalf("write composer.json: %v", err)
				}
			}

			runComposer := !tc.skipComposer && shouldRunComposer(dir)
			if runComposer != tc.wantComposer {
				t.Errorf("runComposer = %v, want %v", runComposer, tc.wantComposer)
			}

			totalSteps := 7
			if runComposer {
				totalSteps++
			}

			if totalSteps != tc.wantTotalStep {
				t.Errorf("totalSteps = %d, want %d", totalSteps, tc.wantTotalStep)
			}
		})
	}
}

func TestShouldRunComposerRequiresARegularFile(t *testing.T) {
	dir := t.TempDir()

	if shouldRunComposer(dir) {
		t.Error("no composer.json, want false")
	}

	// A directory named composer.json is not a manifest.
	if err := os.Mkdir(filepath.Join(dir, "composer.json"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if shouldRunComposer(dir) {
		t.Error("composer.json is a directory, want false")
	}
}
