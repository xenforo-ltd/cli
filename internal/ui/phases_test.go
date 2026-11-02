package ui

import (
	"strings"
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

func TestParseInstallImportMessageHandlesCaseMapping(t *testing.T) {
	// Lowercasing "İ" expands to more bytes, so offsets taken from the
	// lowercased string must not be applied to the original.
	line := strings.Repeat("İ", 20) + " importing master data (phrases: 35%)"
	if got := parseInstallImportMessage(line); got != "importing phrases (35%)" {
		t.Fatalf("message = %q", got)
	}
}
