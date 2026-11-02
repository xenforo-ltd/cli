package ui

import (
	"testing"
)

func TestPhaseTrackerWriterPreservesNonASCII(t *testing.T) {
	writer := NewPhaseTrackerWriter(NewSpinner("Installing"), "Installing XenForo", InstallPhaseRules())

	line := "Importing master data (phrases: 20%) café ✓"
	if _, err := writer.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}

	tail := writer.TailLines()
	if len(tail) != 1 || tail[0] != line {
		t.Fatalf("tail = %q, want %q", tail, line)
	}
}
