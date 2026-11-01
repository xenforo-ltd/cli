package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/xenforo-ltd/cli/internal/ui"
)

type phaseRule struct {
	contains []string
	message  string
}

type phaseTrackerWriter struct {
	mu          sync.Mutex
	spinner     *ui.Spinner
	baseMessage string
	rules       []phaseRule
	pending     string
	lastMessage string
	tail        []string
	tailMax     int
}

func newPhaseTrackerWriter(spinner *ui.Spinner, baseMessage string, rules []phaseRule) *phaseTrackerWriter {
	return &phaseTrackerWriter{
		spinner:     spinner,
		baseMessage: baseMessage,
		rules:       rules,
		tailMax:     25,
	}
}

func (w *phaseTrackerWriter) Write(p []byte) (int, error) {
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

func (w *phaseTrackerWriter) TailLines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]string, len(w.tail))
	copy(out, w.tail)

	return out
}

func (w *phaseTrackerWriter) processLine(line string) {
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

func dockerStartPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"pulling", "pull complete", "downloaded", "extracting"}, message: "pulling images"},
		{contains: []string{"building", "load build", "cached", "exporting", "writing image"}, message: "building services"},
		{contains: []string{"creating", "recreating", "starting", "started", "running"}, message: "starting containers"},
	}
}

func composerPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"loading composer repositories"}, message: "loading repositories"},
		{contains: []string{"updating dependencies"}, message: "updating dependencies"},
		{contains: []string{"installing dependencies"}, message: "installing dependencies"},
		{contains: []string{"generating autoload"}, message: "finalizing"},
	}
}

func installPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"installing", "initializing"}, message: "initializing"},
		{contains: []string{"importing", "master data", "phrases", "templates"}, message: "importing data"},
		{contains: []string{"rebuilding", "caches"}, message: "rebuilding caches"},
		{contains: []string{"installation complete", "install complete", "completed successfully", "setup complete"}, message: "finalizing"},
	}
}

func printHiddenOutputTail(title string, lines []string) {
	if len(lines) == 0 {
		return
	}

	ui.Println(ui.Indent2 + ui.Dim.Render("── "+title+" (last "+ui.Plural(len(lines), "line", "lines")+") ──"))

	for _, line := range lines {
		ui.Printf("%s%s\n", ui.Indent2, ui.Dim.Render(line))
	}
}
