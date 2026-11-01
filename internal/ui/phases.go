package ui

import (
	"fmt"
	"strings"
	"sync"
)

type PhaseRule struct {
	contains []string
	message  string
}

type PhaseTrackerWriter struct {
	mu          sync.Mutex
	spinner     *Spinner
	baseMessage string
	rules       []PhaseRule
	pending     string
	lastMessage string
	tail        []string
	tailMax     int
}

func NewPhaseTrackerWriter(spinner *Spinner, baseMessage string, rules []PhaseRule) *PhaseTrackerWriter {
	return &PhaseTrackerWriter{
		spinner:     spinner,
		baseMessage: baseMessage,
		rules:       rules,
		tailMax:     25,
	}
}

func (w *PhaseTrackerWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range p {
		if b == '\n' || b == '\r' {
			w.processLine(w.pending)
			w.pending = ""

			continue
		}

		w.pending += string(b)
	}

	return len(p), nil
}

func (w *PhaseTrackerWriter) TailLines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]string, len(w.tail))
	copy(out, w.tail)

	return out
}

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
			w.lastMessage = importMessage
			w.spinner.UpdateMessage(fmt.Sprintf("%s (%s)", w.baseMessage, importMessage))

			return
		}
	}

	lower := strings.ToLower(trimmed)
	for _, rule := range w.rules {
		if containsAny(lower, rule.contains) {
			if rule.message != "" && rule.message != w.lastMessage {
				w.lastMessage = rule.message
				w.spinner.UpdateMessage(fmt.Sprintf("%s (%s)", w.baseMessage, rule.message))
			}

			return
		}
	}
}

func parseInstallImportMessage(line string) string {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "import") && !strings.Contains(lower, "master data") {
		return ""
	}

	const marker = "master data ("

	idx := strings.Index(lower, marker)
	if idx >= 0 {
		after := line[idx+len(marker):]

		end := strings.Index(after, ")")
		if end < 0 {
			end = len(after)
		}

		inside := strings.TrimSpace(after[:end])
		if inside == "" {
			return "importing data"
		}

		partsCount := 2
		parts := strings.SplitN(inside, ":", partsCount)

		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			return "importing data"
		}

		if len(parts) == partsCount {
			percent := strings.TrimSpace(parts[1])
			if percent != "" {
				return fmt.Sprintf("importing %s (%s)", name, percent)
			}
		}

		return "importing " + name
	}

	return "importing data"
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}

	return false
}

func DockerStartPhaseRules() []PhaseRule {
	return []PhaseRule{
		{contains: []string{"pulling", "pull complete", "downloaded", "extracting"}, message: "pulling images"},
		{contains: []string{"building", "load build", "cached", "exporting", "writing image"}, message: "building services"},
		{contains: []string{"creating", "recreating", "starting", "started", "running"}, message: "starting containers"},
	}
}

func ComposerPhaseRules() []PhaseRule {
	return []PhaseRule{
		{contains: []string{"loading composer repositories"}, message: "loading repositories"},
		{contains: []string{"updating dependencies"}, message: "updating dependencies"},
		{contains: []string{"installing dependencies"}, message: "installing dependencies"},
		{contains: []string{"generating autoload"}, message: "finalizing"},
	}
}

func InstallPhaseRules() []PhaseRule {
	return []PhaseRule{
		{contains: []string{"installing", "initializing"}, message: "initializing"},
		{contains: []string{"importing", "master data", "phrases", "templates"}, message: "importing data"},
		{contains: []string{"rebuilding", "caches"}, message: "rebuilding caches"},
		{contains: []string{"installation complete", "install complete", "completed successfully", "setup complete"}, message: "finalizing"},
	}
}

func PrintHiddenOutputTail(title string, lines []string) {
	if len(lines) == 0 {
		return
	}

	Println(Indent2 + Dim.Render("── "+title+" (last "+Plural(len(lines), "line", "lines")+") ──"))

	for _, line := range lines {
		Printf("%s%s\n", Indent2, Dim.Render(line))
	}
}
