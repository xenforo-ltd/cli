package ui

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// phaseRule maps output substrings to a spinner phase message.
type phaseRule struct {
	contains []string
	message  string
}

// PhaseTrackerWriter tracks complete output lines and updates a spinner for
// recognized phases. Its writes and tail snapshots are synchronized.
type PhaseTrackerWriter struct {
	mu          sync.Mutex
	spinner     *Spinner
	baseMessage string
	rules       []phaseRule
	pending     strings.Builder
	lastMessage string
	tail        []string
	tailMax     int
}

// NewPhaseTrackerWriter creates a tracker retaining the last 25 nonempty lines.
func NewPhaseTrackerWriter(spinner *Spinner, baseMessage string, rules []phaseRule) *PhaseTrackerWriter {
	return &PhaseTrackerWriter{
		spinner:     spinner,
		baseMessage: baseMessage,
		rules:       rules,
		tailMax:     25,
	}
}

// Write buffers output and processes lines terminated by a carriage return or newline.
func (w *PhaseTrackerWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range p {
		if b == '\n' || b == '\r' {
			w.processLine(w.pending.String())
			w.pending.Reset()

			continue
		}

		w.pending.WriteByte(b)
	}

	return len(p), nil
}

// TailLines returns a copy of the retained complete, nonempty lines.
func (w *PhaseTrackerWriter) TailLines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return slices.Clone(w.tail)
}

// processLine retains a nonempty line and updates the matching phase.
func (w *PhaseTrackerWriter) processLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	w.tail = append(w.tail, trimmed)
	if len(w.tail) > w.tailMax {
		w.tail = w.tail[len(w.tail)-w.tailMax:]
	}

	if strings.HasPrefix(w.baseMessage, "Installing XenForo") {
		if importMessage := parseInstallImportMessage(trimmed); importMessage != "" && importMessage != w.lastMessage {
			w.setPhase(importMessage)

			return
		}
	}

	lower := strings.ToLower(trimmed)
	for _, rule := range w.rules {
		if containsAny(lower, rule.contains) {
			if rule.message != "" && rule.message != w.lastMessage {
				w.setPhase(rule.message)
			}

			return
		}
	}
}

// setPhase records the phase and updates the spinner message.
func (w *PhaseTrackerWriter) setPhase(message string) {
	w.lastMessage = message
	w.spinner.UpdateMessage(fmt.Sprintf("%s (%s)", w.baseMessage, message))
}

// parseInstallImportMessage extracts an installation import phase from a line.
func parseInstallImportMessage(line string) string {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "import") && !strings.Contains(lower, "master data") {
		return ""
	}

	const marker = "master data ("

	_, after, ok := strings.Cut(lower, marker)
	if !ok {
		return "importing data"
	}

	inside, _, _ := strings.Cut(after, ")")
	name, percent, _ := strings.Cut(inside, ":")
	name = strings.TrimSpace(name)
	if name == "" {
		return "importing data"
	}

	if percent = strings.TrimSpace(percent); percent != "" {
		return fmt.Sprintf("importing %s (%s)", name, percent)
	}

	return "importing " + name
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, needles []string) bool {
	return slices.ContainsFunc(needles, func(n string) bool { return strings.Contains(s, n) })
}

// DockerStartPhaseRules returns rules for Docker startup output.
func DockerStartPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"pulling", "pull complete", "downloaded", "extracting"}, message: "pulling images"},
		{contains: []string{"building", "load build", "cached", "exporting", "writing image"}, message: "building services"},
		{contains: []string{"creating", "recreating", "starting", "started", "running"}, message: "starting containers"},
	}
}

// ComposerPhaseRules returns rules for Composer output.
func ComposerPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"loading composer repositories"}, message: "loading repositories"},
		{contains: []string{"updating dependencies"}, message: "updating dependencies"},
		{contains: []string{"installing dependencies"}, message: "installing dependencies"},
		{contains: []string{"generating autoload"}, message: "finalizing"},
	}
}

// InstallPhaseRules returns rules for XenForo installation output.
func InstallPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"installing", "initializing"}, message: "initializing"},
		{contains: []string{"importing", "master data", "phrases", "templates"}, message: "importing data"},
		{contains: []string{"rebuilding", "caches"}, message: "rebuilding caches"},
		{contains: []string{"installation complete", "install complete", "completed successfully", "setup complete"}, message: "finalizing"},
	}
}

// PrintHiddenOutputTail prints a titled tail of hidden output when it is nonempty.
func PrintHiddenOutputTail(title string, lines []string) {
	if len(lines) == 0 {
		return
	}

	Println(Indent2 + Dim.Render("── "+title+" (last "+Plural(len(lines), "line", "lines")+") ──"))

	for _, line := range lines {
		Printf("%s%s\n", Indent2, Dim.Render(line))
	}
}
