// Package ui provides formatting and styling utilities for console output.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
)

// ColorPrimary is the primary accent color.
var (
	hasDark   = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	lightDark = lipgloss.LightDark(hasDark)

	ColorPrimary   = lipgloss.Blue
	ColorSecondary = lipgloss.Magenta
	ColorAccent    = lipgloss.Cyan
	ColorSuccess   = lipgloss.Green
	ColorWarning   = lipgloss.Yellow
	ColorError     = lipgloss.Red
	ColorInfo      = lipgloss.Cyan
	ColorSubtle    = lightDark(lipgloss.Color("#585858"), lipgloss.Color("#BCBCBC"))
)

const ansiClearLine = "\r\033[2K"

// Predefined styles for consistent use across commands.
var (
	Bold      = lipgloss.NewStyle().Bold(true)
	Dim       = lipgloss.NewStyle().Faint(true)                        // Terminal's native faint/dim
	Muted     = lipgloss.NewStyle().Foreground(ColorSubtle)            // Adaptive subtle color
	Label     = lipgloss.NewStyle().Foreground(ColorSubtle)            // For labels in key-value pairs
	Secondary = lipgloss.NewStyle().Foreground(ColorSubtle).Bold(true) // Secondary emphasis (e.g., table headers)

	Success = lipgloss.NewStyle().Foreground(ColorSuccess)
	Warning = lipgloss.NewStyle().Foreground(ColorWarning)
	Error   = lipgloss.NewStyle().Foreground(ColorError)
	Info    = lipgloss.NewStyle().Foreground(ColorInfo)

	SuccessBold = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	WarningBold = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	ErrorBold   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	InfoBold    = lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)

	Header = lipgloss.NewStyle().Bold(true).Underline(true)

	Command = lipgloss.NewStyle().Foreground(ColorAccent)
	Path    = lipgloss.NewStyle().Foreground(ColorSecondary)
	URL     = lipgloss.NewStyle().Foreground(ColorPrimary).Underline(true)
	Version = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
)

// Indent1 is one level of indentation (2 spaces).
const (
	Indent1 = "  "
	Indent2 = "    "
)

// SymbolSuccess is the success symbol.
const (
	SymbolSuccess = "✓"
	SymbolWarning = "!"
	SymbolError   = "✗"
	SymbolInfo    = "•"
	SymbolSkipped = "-"
	SymbolPending = "○"
	SymbolArrow   = "→"
	SymbolBullet  = "•"
	SymbolCheck   = "✓"
	SymbolCross   = "✗"
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
		return Success.Render(SymbolSuccess)
	case "warning", "warn":
		return Warning.Render(SymbolWarning)
	case "error", "fail":
		return Error.Render(SymbolError)
	case "info":
		return Info.Render(SymbolInfo)
	case "skipped", "skip":
		return Dim.Render(SymbolSkipped)
	case "pending":
		return Dim.Render(SymbolPending)
	default:
		return Dim.Render("?")
	}
}

// Plural returns the count with the correct singular/plural noun.
func Plural(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// FormatDate renders a date for table cells.
func FormatDate(t time.Time) string { return t.Format("2006-01-02") }

// FormatDateTime renders a timestamp for key-value output.
func FormatDateTime(t time.Time) string { return t.Format("2006-01-02 15:04") }

// IsTerminal reports whether f is an interactive terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ShortHome abbreviates the home directory prefix of a path to ~.
var ShortHome = func(path string) string {
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

// isTTY records whether stdout is an interactive terminal, gating spinner
// and progress-bar animation.
var isTTY = IsTerminal(os.Stdout)

// Step returns a formatted progress step indicator.
func Step(current, total int) string {
	return Info.Render(fmt.Sprintf("[%d/%d]", current, total))
}

// StepWithLabel returns a progress step with a label.
func StepWithLabel(current, total int, label string) string {
	return fmt.Sprintf("%s %s", Step(current, total), Bold.Render(label))
}

// Indent indents all non-empty lines of text by the specified number of spaces.
func Indent(s string, spaces int) string {
	indent := strings.Repeat(" ", spaces)

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}

	return strings.Join(lines, "\n")
}

// IndentLines indents each line of text by the specified number of spaces.
func IndentLines(lines []string, spaces int) []string {
	indent := strings.Repeat(" ", spaces)

	result := make([]string, len(lines))
	for i, line := range lines {
		if line != "" {
			result[i] = indent + line
		} else {
			result[i] = line
		}
	}

	return result
}

// Separator returns a horizontal separator line.
func Separator(width int) string {
	if width <= 0 {
		width = 60
	}

	return Dim.Render(strings.Repeat("─", width))
}

// KeyValue returns a formatted key-value pair.
func KeyValue(key, value string) string {
	return fmt.Sprintf("%s %s", Label.Render(key+":"), value)
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
		fmt.Fprintf(&sb, "%s%s%s  %s\n", indent, Label.Render(p.Key+":"), padding, p.Value)
	}

	return sb.String()
}

// List formats a slice of strings as a bulleted list.
func List(items []string) string {
	var sb strings.Builder
	for _, item := range items {
		fmt.Fprintf(&sb, "  %s %s\n", Dim.Render(SymbolBullet), item)
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// Spinner provides a simple terminal spinner.
type Spinner struct {
	mu       sync.Mutex
	frames   []string
	interval time.Duration
	message  string
	writer   io.Writer
	done     chan struct{}
	stopped  chan struct{} // closed by the animation goroutine as it exits
	running  bool
	plain    bool // true when Start ran without a TTY: no animation, no Stop line
	frameIdx int
}

// SpinnerFrames are the animation frames for the spinner.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:   SpinnerFrames,
		interval: 80 * time.Millisecond,
		message:  message,
		writer:   os.Stdout,
		done:     make(chan struct{}),
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
			frame := Info.Render(s.frames[s.frameIdx%len(s.frames)])
			lipgloss.Fprint(s.writer, ansiClearLine)
			lipgloss.Fprintf(s.writer, "%s %s", frame, msg)
			s.frameIdx++
			s.mu.Unlock()

			// Waiting on the ticker and done together keeps Stop responsive:
			// a plain sleep would make it block for up to a full interval.
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop stops the spinner and clears the line. It is a no-op if Start ran
// without a TTY (the message was already printed once, plainly).
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

	// Wait for the animation goroutine to exit before returning, so a
	// subsequent Start — or any output printed after Stop — cannot be
	// overwritten by a stale frame from the old goroutine.
	<-stopped

	s.mu.Lock()
	defer s.mu.Unlock()

	lipgloss.Fprint(s.writer, ansiClearLine)
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

// SpinnerOutputWriter writes to a writer while managing spinner output.
type SpinnerOutputWriter struct {
	spinner *Spinner
	writer  io.Writer
}

// NewSpinnerOutputWriter creates a writer that coordinates with a spinner.
func NewSpinnerOutputWriter(spinner *Spinner, writer io.Writer) io.Writer {
	return &SpinnerOutputWriter{
		spinner: spinner,
		writer:  writer,
	}
}

func (w *SpinnerOutputWriter) Write(p []byte) (int, error) {
	if w.spinner == nil {
		n, err := w.writer.Write(p)
		if err != nil {
			return n, fmt.Errorf("failed to write spinner output: %w", err)
		}

		return n, nil
	}

	w.spinner.mu.Lock()
	defer w.spinner.mu.Unlock()

	repaint := isTTY && w.spinner.running && !w.spinner.plain

	if repaint {
		lipgloss.Fprint(w.spinner.writer, ansiClearLine)
	}

	n, err := w.writer.Write(p)
	if err != nil {
		return n, fmt.Errorf("failed to write spinner output: %w", err)
	}

	if repaint {
		lipgloss.Fprint(w.spinner.writer, "\n")
		frame := Info.Render(w.spinner.frames[w.spinner.frameIdx%len(w.spinner.frames)])
		lipgloss.Fprint(w.spinner.writer, ansiClearLine)
		lipgloss.Fprintf(w.spinner.writer, "%s %s", frame, w.spinner.message)
		w.spinner.frameIdx++
	}

	return n, nil
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

// Increment increments the progress bar by the given amount.
func (p *ProgressBar) Increment(amount int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current += amount
	if p.current > p.total {
		p.current = p.total
	}

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
		Info.Render(pctStr),
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

// PrintSuccess prints a success message.
func PrintSuccess(message string) {
	lipgloss.Printf("%s %s\n", StatusIcon("success"), message)
}

// PrintWarning prints a warning message.
func PrintWarning(message string) {
	lipgloss.Printf("%s %s\n", StatusIcon("warning"), message)
}

// PrintError prints an error message.
func PrintError(message string) {
	lipgloss.Printf("%s %s\n", StatusIcon("error"), message)
}

// PrintInfo prints an info message.
func PrintInfo(message string) {
	lipgloss.Printf("%s %s\n", StatusIcon("info"), message)
}

// PrintStep prints a step message.
func PrintStep(current, total int, message string) {
	lipgloss.Println(StepWithLabel(current, total, message))
}

// PrintSubstep prints an indented substep message with arrow.
func PrintSubstep(message string) {
	lipgloss.Printf("%s%s %s\n", Indent2, Dim.Render(SymbolArrow), message)
}

// PrintDetail prints an indented detail message (dimmed).
func PrintDetail(message string) {
	lipgloss.Printf("%s%s\n", Indent2, Dim.Render(message))
}

// PrintKeyValue prints a key-value pair.
func PrintKeyValue(key, value string) {
	lipgloss.Println(KeyValue(key, value))
}

// PrintHint prints an actionable next step: "  → Run xf auth login to ...".
// Callers style embedded commands themselves.
func PrintHint(text string) {
	lipgloss.Printf("%s%s %s\n", Indent1, Dim.Render(SymbolArrow), text)
}

// PrintEmpty prints a standard empty-result line.
func PrintEmpty(message string) {
	lipgloss.Printf("%s %s\n", StatusIcon("info"), message)
}

// SuccessBox prints a success message with optional key-value details.
// This replaces heavy double-line separators with a cleaner format.
func SuccessBox(message string, details []KVPair) {
	lipgloss.Printf("%s %s\n", StatusIcon("success"), SuccessBold.Render(message))

	if len(details) > 0 {
		lipgloss.Println()
		PrintKeyValuePadded(details)
	}
}

// InfoBox prints an info message with optional key-value details.
func InfoBox(message string, details []KVPair) {
	lipgloss.Printf("%s %s\n", StatusIcon("info"), Bold.Render(message))

	if len(details) > 0 {
		lipgloss.Println()
		PrintKeyValuePadded(details)
	}
}

// WarningBox prints a warning message with optional key-value details.
func WarningBox(message string, details []KVPair) {
	lipgloss.Printf("%s %s\n", StatusIcon("warning"), WarningBold.Render(message))

	if len(details) > 0 {
		lipgloss.Println()
		PrintKeyValuePadded(details)
	}
}

// ErrorBox prints an error message with optional key-value details.
func ErrorBox(message string, details []KVPair) {
	lipgloss.Printf("%s %s\n", StatusIcon("error"), ErrorBold.Render(message))

	if len(details) > 0 {
		lipgloss.Println()
		PrintKeyValuePadded(details)
	}
}
