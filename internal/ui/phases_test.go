package ui

import "testing"

func TestParseInstallImportMessage(t *testing.T) {
	if got := parseInstallImportMessage("Importing master data (phrases: 35%)"); got != "importing phrases (35%)" {
		t.Fatalf("unexpected message: %q", got)
	}

	if got := parseInstallImportMessage("some line without marker"); got != "" {
		t.Fatalf("expected empty message, got %q", got)
	}

	if got := parseInstallImportMessage("importing master data ("); got != "importing data" {
		t.Fatalf("expected fallback import message, got %q", got)
	}
}

func TestContainsAnyAndPhaseRules(t *testing.T) {
	if !containsAny("starting containers", []string{"pull", "starting"}) {
		t.Fatal("expected match")
	}

	if containsAny("starting containers", []string{"build", "cached"}) {
		t.Fatal("did not expect match")
	}

	if len(DockerStartPhaseRules()) == 0 || len(InstallPhaseRules()) == 0 {
		t.Fatal("expected non-empty phase rules")
	}
}

func TestPhaseTrackerWriterProcessLine(t *testing.T) {
	spinner := NewSpinner("base")
	writer := NewPhaseTrackerWriter(spinner, "Installing XenForo", InstallPhaseRules())

	if _, err := writer.Write([]byte("Importing master data (phrases: 20%)\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	tail := writer.TailLines()
	if len(tail) == 0 {
		t.Fatal("expected tail lines")
	}
}
