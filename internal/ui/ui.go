// Package ui provides formatting and styling utilities for console output.
package ui

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
)

var (
	// lightDark selects a color for the terminal background.
	lightDark = lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

	// colorPrimary is the primary display color.
	colorPrimary = lipgloss.Blue
	// colorSecondary is the secondary display color.
	colorSecondary = lipgloss.Magenta
	// colorAccent is the accent display color.
	colorAccent = lipgloss.Cyan
	// colorSuccess is the success display color.
	colorSuccess = lipgloss.Green
	// colorWarning is the warning display color.
	colorWarning = lipgloss.Yellow
	// colorError is the error display color.
	colorError = lipgloss.Red
	// colorInfo is the informational display color.
	colorInfo = lipgloss.Cyan
	// colorSubtle adapts the subtle display color to the background.
	colorSubtle = lightDark(lipgloss.Color("#585858"), lipgloss.Color("#BCBCBC"))
)

// ansiClearLine clears the current terminal line.
const ansiClearLine = "\r\033[2K"

var (
	// Bold emphasizes text.
	Bold = lipgloss.NewStyle().Bold(true)
	// Dim uses the terminal's faint style.
	Dim = lipgloss.NewStyle().Faint(true)
	// Muted renders text in an adaptive subtle color.
	Muted = lipgloss.NewStyle().Foreground(colorSubtle)
	// labelStyle renders key-value labels in an adaptive subtle color.
	labelStyle = lipgloss.NewStyle().Foreground(colorSubtle)

	// Success styles successful status output.
	Success = lipgloss.NewStyle().Foreground(colorSuccess)
	// Warning styles warning status output.
	Warning = lipgloss.NewStyle().Foreground(colorWarning)
	// Error styles error status output.
	Error = lipgloss.NewStyle().Foreground(colorError)
	// infoStyle styles informational status output.
	infoStyle = lipgloss.NewStyle().Foreground(colorInfo)

	// successBold emphasizes successful output.
	successBold = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	// ErrorBold emphasizes error output.
	ErrorBold = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	// Header emphasizes and underlines headings.
	Header = lipgloss.NewStyle().Bold(true).Underline(true)

	// Command styles command text.
	Command = lipgloss.NewStyle().Foreground(colorAccent)
	// Path styles filesystem paths.
	Path = lipgloss.NewStyle().Foreground(colorSecondary)
	// URL styles URLs.
	URL = lipgloss.NewStyle().Foreground(colorPrimary).Underline(true)
	// Version styles version text.
	Version = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

const (
	// Indent1 is one level of indentation.
	Indent1 = "  "
	// Indent2 is two levels of indentation.
	Indent2 = "    "
)

const (
	// symbolSuccess indicates success.
	symbolSuccess = "✓"
	// symbolWarning indicates a warning.
	symbolWarning = "!"
	// SymbolError indicates an error.
	SymbolError = "✗"
	// symbolInfo indicates informational output.
	symbolInfo = "•"
	// symbolSkipped indicates a skipped operation.
	symbolSkipped = "-"
	// symbolPending indicates a pending operation.
	symbolPending = "○"
	// SymbolArrow indicates a next step.
	SymbolArrow = "→"
	// SymbolBullet is the informational bullet symbol.
	SymbolBullet = symbolInfo
)

// Println is a wrapper around lipgloss.Println for color-downsampled output.
func Println(v ...any) int {
	n, _ := lipgloss.Println(v...)

	return n
}

// Printf is a wrapper around lipgloss.Printf for color-downsampled output.
func Printf(format string, v ...any) int {
	n, _ := lipgloss.Printf(format, v...)

	return n
}

// StatusIcon renders a colored status symbol.
func StatusIcon(status string) string {
	switch status {
	case "success", "ok":
		return Success.Render(symbolSuccess)
	case "warning", "warn":
		return Warning.Render(symbolWarning)
	case "error", "fail":
		return Error.Render(SymbolError)
	case "info":
		return infoStyle.Render(symbolInfo)
	case "skipped", "skip":
		return Dim.Render(symbolSkipped)
	case "pending":
		return Dim.Render(symbolPending)
	default:
		return Dim.Render("?")
	}
}

// Plural returns the count with the correct singular/plural noun.
func Plural(n int, singular, plural string) string {
	noun := plural
	if n == 1 {
		noun = singular
	}

	return formatCount(n) + " " + noun
}

// FormatDate renders a date for table cells.
func FormatDate(t time.Time) string { return t.Format("2006-01-02") }

// FormatDateTime renders a timestamp for key-value output.
func FormatDateTime(t time.Time) string { return t.Format("2006-01-02 15:04") }

// isTerminal reports whether f is an interactive terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

// ClearScreen clears the visible screen when stdout is a terminal.
func ClearScreen() {
	if isTerminal(os.Stdout) {
		fmt.Fprint(os.Stdout, "\033[H\033[2J")
	}
}

// ShortHome abbreviates the home directory prefix of a path to ~.
func ShortHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}

	if path == home {
		return "~"
	}

	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}

	return path
}

// FileLink links text to an absolute file path using OSC 8 in interactive
// terminals. Redirected output keeps the plain text. Drive-letter paths gain a
// leading slash, and paths beginning with two slashes identify a host.
func FileLink(text, filename string) string {
	if !isTTY {
		return text
	}

	filename = filepath.ToSlash(filename)
	target := url.URL{Scheme: "file", Path: filename}
	switch {
	case strings.HasPrefix(filename, "//"):
		host, path, _ := strings.Cut(filename[2:], "/")
		target.Host, target.Path = host, "/"+path
	case !strings.HasPrefix(filename, "/"):
		target.Path = "/" + filename
	}

	return Path.Hyperlink(target.String()).Render(text)
}

// isTTY records whether stdout is an interactive terminal, gating spinner
// and progress-bar animation and terminal-only hyperlinks.
var isTTY = isTerminal(os.Stdout)

// Step returns a formatted progress step indicator.
func Step(current, total int) string {
	return infoStyle.Render(fmt.Sprintf("[%d/%d]", current, total))
}

// stepWithLabel returns a progress step with a label.
func stepWithLabel(current, total int, label string) string {
	return fmt.Sprintf("%s %s", Step(current, total), Bold.Render(label))
}

// KVPair represents a key-value pair for display.
type KVPair struct {
	Key   string
	Value string
}

// KV creates a key-value pair.
func KV(key, value string) KVPair {
	return KVPair{Key: key, Value: value}
}

// PrintKeyValuePadded prints key-value pairs with aligned values.
func PrintKeyValuePadded(pairs []KVPair) {
	PrintKeyValuePaddedWithIndent(pairs, Indent1)
}

// PrintKeyValuePaddedWithIndent prints key-value pairs with custom indentation.
func PrintKeyValuePaddedWithIndent(pairs []KVPair, indent string) {
	out := renderKeyValuePadded(pairs, indent)
	if out == "" {
		return
	}

	_, _ = lipgloss.Print(out)
}

// renderKeyValuePadded renders key-value pairs with values aligned to the
// widest key, using display width so styled/multi-byte keys don't break
// alignment.
func renderKeyValuePadded(pairs []KVPair, indent string) string {
	if len(pairs) == 0 {
		return ""
	}

	maxKeyLen := 0
	for _, p := range pairs {
		if w := lipgloss.Width(p.Key); w > maxKeyLen {
			maxKeyLen = w
		}
	}

	var sb strings.Builder
	for _, p := range pairs {
		padding := strings.Repeat(" ", maxKeyLen-lipgloss.Width(p.Key))
		fmt.Fprintf(&sb, "%s%s%s  %s\n", indent, labelStyle.Render(p.Key+":"), padding, p.Value)
	}

	return sb.String()
}

// Spinner provides a synchronized terminal spinner. The animation goroutine
// signals its exit on stopping; nonterminal starts print once without animation.
type Spinner struct {
	mu       sync.Mutex
	interval time.Duration
	message  string
	writer   io.Writer
	done     chan struct{}
	stopped  chan struct{}
	running  bool
	plain    bool
	frameIdx int
}

// spinnerFrames are the animation frames for the spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		interval: 80 * time.Millisecond,
		message:  message,
		writer:   os.Stdout,
	}
}

// Start begins the spinner animation. When stdout is not an interactive
// terminal, it prints the message once and returns without animating.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}

	if !isTTY {
		s.plain = true
		s.running = true
		msg := s.message
		s.mu.Unlock()
		lipgloss.Fprintf(s.writer, "%s\n", msg)
		return
	}

	s.running = true
	s.done = make(chan struct{})
	s.stopped = make(chan struct{})
	done, stopped := s.done, s.stopped
	s.mu.Unlock()

	go func() {
		defer close(stopped)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			s.mu.Lock()
			msg := s.message
			frame := infoStyle.Render(spinnerFrames[s.frameIdx%len(spinnerFrames)])
			lipgloss.Fprint(s.writer, ansiClearLine)
			lipgloss.Fprintf(s.writer, "%s %s", frame, msg)
			s.frameIdx++
			s.mu.Unlock()

			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop stops the spinner and clears the line. It is a no-op if Start ran
// without a TTY (the message was already printed once, plainly). It waits for
// the animation goroutine to exit before returning and clearing the line.
func (s *Spinner) Stop() {
	s.mu.Lock()

	if !s.running {
		s.mu.Unlock()
		return
	}

	s.running = false

	if s.plain {
		s.mu.Unlock()
		return
	}

	stopped := s.stopped
	close(s.done)
	s.mu.Unlock()

	<-stopped

	s.mu.Lock()
	defer s.mu.Unlock()

	lipgloss.Fprint(s.writer, ansiClearLine)
}

// Animating reports whether the spinner is actively animating. It returns
// false when Start ran without a TTY (the message was printed once, plainly)
// and when the spinner is not running.
func (s *Spinner) Animating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running && !s.plain
}

// StopWithMessage stops the spinner and prints a final message.
func (s *Spinner) StopWithMessage(status, message string) {
	s.Stop()
	lipgloss.Fprintf(s.writer, "%s %s\n", StatusIcon(status), message)
}

// UpdateMessage updates the spinner message.
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.message = message
}

// ProgressBar displays download or operation progress.
type ProgressBar struct {
	mu      sync.Mutex
	total   int64
	current int64
	width   int
	message string
	writer  io.Writer
}

// NewProgressBar creates a new progress bar.
func NewProgressBar(total int64, message string) *ProgressBar {
	return &ProgressBar{
		total:   total,
		width:   40,
		message: message,
		writer:  os.Stdout,
	}
}

// Update updates the progress bar.
func (p *ProgressBar) Update(current int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	p.render()
}

// Finish completes the progress bar.
func (p *ProgressBar) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = p.total
	p.render()

	if isTTY {
		lipgloss.Fprintln(p.writer)
	}
}

// Abandon ends an unfinished progress bar, clearing its line. Use this when the
// operation failed: Finish would paint the bar at 100%, reporting a partial or
// failed transfer as complete.
func (p *ProgressBar) Abandon() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if isTTY {
		lipgloss.Fprint(p.writer, ansiClearLine)
	}
}

// render draws the current progress when the total is positive and stdout is a terminal.
func (p *ProgressBar) render() {
	if p.total <= 0 || !isTTY {
		return
	}

	percent := float64(p.current) / float64(p.total)
	filled := int(percent * float64(p.width))
	empty := p.width - filled

	bar := Success.Render(strings.Repeat("█", filled)) +
		Dim.Render(strings.Repeat("░", empty))

	pctStr := fmt.Sprintf("%3.0f%%", percent*100)
	sizeStr := fmt.Sprintf("%s / %s", FormatBytes(p.current), FormatBytes(p.total))

	lipgloss.Fprintf(p.writer, "%s%s %s %s %s",
		ansiClearLine,
		p.message,
		bar,
		infoStyle.Render(pctStr),
		Dim.Render(sizeStr))
}

// FormatBytes formats a byte count as a human-readable string.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// printStatus prints a message with its status icon.
func printStatus(status, message string) {
	lipgloss.Printf("%s %s\n", StatusIcon(status), message)
}

// PrintSuccess prints a success message.
func PrintSuccess(message string) { printStatus("success", message) }

// PrintWarning prints a warning message.
func PrintWarning(message string) { printStatus("warning", message) }

// PrintError prints an error message.
func PrintError(message string) { printStatus("error", message) }

// PrintInfo prints an info message.
func PrintInfo(message string) { printStatus("info", message) }

// PrintStep prints a step message.
func PrintStep(current, total int, message string) {
	lipgloss.Println(stepWithLabel(current, total, message))
}

// PrintSubstep prints an indented substep message with arrow.
func PrintSubstep(message string) {
	lipgloss.Printf("%s%s %s\n", Indent2, Dim.Render(SymbolArrow), message)
}

// PrintDetail prints an indented detail message (dimmed).
func PrintDetail(message string) {
	lipgloss.Printf("%s%s\n", Indent2, Dim.Render(message))
}

// PrintCause prints the underlying error beneath a status line, but only under
// verbose output.
//
// Status lines stay readable by default, while the detail needed to diagnose a
// failure is still one flag away rather than discarded.
func PrintCause(err error, verbose bool) {
	if err == nil || !verbose {
		return
	}

	PrintDetail(err.Error())
}

// PrintHint prints an actionable next step: "  → Run xf auth login to ...".
// Callers style embedded commands themselves.
func PrintHint(text string) {
	lipgloss.Printf("%s%s %s\n", Indent1, Dim.Render(SymbolArrow), text)
}

// PrintEmpty prints a standard empty-result line.
func PrintEmpty(message string) { printStatus("info", message) }

// printBox prints a status message and optional key-value details.
func printBox(status, message string, details []KVPair) {
	printStatus(status, message)

	if len(details) > 0 {
		lipgloss.Println()
		PrintKeyValuePadded(details)
	}
}

// SuccessBox prints a success message with optional key-value details.
func SuccessBox(message string, details []KVPair) {
	printBox("success", successBold.Render(message), details)
}

// InfoBox prints an info message with optional key-value details.
func InfoBox(message string, details []KVPair) {
	printBox("info", Bold.Render(message), details)
}
