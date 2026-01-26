package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorPrimary   = lipgloss.Color("4")
	ColorSecondary = lipgloss.Color("5")
	ColorAccent    = lipgloss.Color("6")
	ColorSuccess   = lipgloss.Color("2")
	ColorWarning   = lipgloss.Color("3")
	ColorError     = lipgloss.Color("1")
	ColorInfo      = lipgloss.Color("6")
	ColorSubtle    = lipgloss.AdaptiveColor{Light: "240", Dark: "250"}
)

const ansiClearLine = "\r\033[2K"

// Predefined styles for consistent use across commands.
var (
	Bold      = lipgloss.NewStyle().Bold(true)
	Italic    = lipgloss.NewStyle().Italic(true)
	Underline = lipgloss.NewStyle().Underline(true)
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

	Header    = lipgloss.NewStyle().Bold(true).Underline(true)
	Subheader = lipgloss.NewStyle().Bold(true)

	Command = lipgloss.NewStyle().Foreground(ColorAccent)
	Path    = lipgloss.NewStyle().Foreground(ColorSecondary)
	URL     = lipgloss.NewStyle().Foreground(ColorPrimary).Underline(true)
	Version = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
)

const (
	Indent1 = "  "
	Indent2 = "    "
)

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

func Step(current, total int) string {
	return Info.Render(fmt.Sprintf("[%d/%d]", current, total))
}

func StepWithLabel(current, total int, label string) string {
	return fmt.Sprintf("%s %s", Step(current, total), Bold.Render(label))
}

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

func Separator(width int) string {
	if width <= 0 {
		width = 60
	}
	return Dim.Render(strings.Repeat("─", width))
}

func DoubleSeparator(width int) string {
	if width <= 0 {
		width = 60
	}
	return Dim.Render(strings.Repeat("═", width))
}

func Box(title, content string) string {
	var sb strings.Builder

	lines := strings.Split(content, "\n")
	maxWidth := len(title)
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}
	maxWidth += 4 // padding

	sb.WriteString(Dim.Render("┌" + strings.Repeat("─", maxWidth) + "┐"))
	sb.WriteString("\n")

	if title != "" {
		padding := maxWidth - len(title) - 1
		sb.WriteString(Dim.Render("│ "))
		sb.WriteString(Bold.Render(title))
		sb.WriteString(strings.Repeat(" ", padding))
		sb.WriteString(Dim.Render("│"))
		sb.WriteString("\n")
		sb.WriteString(Dim.Render("├" + strings.Repeat("─", maxWidth) + "┤"))
		sb.WriteString("\n")
	}

	for _, line := range lines {
		padding := maxWidth - len(line) - 1
		sb.WriteString(Dim.Render("│ "))
		sb.WriteString(line)
		sb.WriteString(strings.Repeat(" ", padding))
		sb.WriteString(Dim.Render("│"))
		sb.WriteString("\n")
	}

	sb.WriteString(Dim.Render("└" + strings.Repeat("─", maxWidth) + "┘"))

	return sb.String()
}

func KeyValue(key, value string) string {
	return fmt.Sprintf("%s %s", Label.Render(key+":"), value)
}

func KeyValueBold(key, value string) string {
	return fmt.Sprintf("%s %s", Bold.Render(key+":"), value)
}

type KVPair struct {
	Key   string
	Value string
}

func KV(key, value string) KVPair {
	return KVPair{Key: key, Value: value}
}

func PrintKeyValuePadded(pairs []KVPair) {
	if len(pairs) == 0 {
		return
	}

	maxKeyLen := 0
	for _, p := range pairs {
		if len(p.Key) > maxKeyLen {
			maxKeyLen = len(p.Key)
		}
	}

	for _, p := range pairs {
		padding := strings.Repeat(" ", maxKeyLen-len(p.Key))
		fmt.Printf("%s%s%s  %s\n", Indent1, Label.Render(p.Key+":"), padding, p.Value)
	}
}

func PrintKeyValuePaddedWithIndent(pairs []KVPair, indent string) {
	if len(pairs) == 0 {
		return
	}

	maxKeyLen := 0
	for _, p := range pairs {
		if len(p.Key) > maxKeyLen {
			maxKeyLen = len(p.Key)
		}
	}

	for _, p := range pairs {
		padding := strings.Repeat(" ", maxKeyLen-len(p.Key))
		fmt.Printf("%s%s%s  %s\n", indent, Label.Render(p.Key+":"), padding, p.Value)
	}
}

// List formats a slice of strings as a bulleted list.
func List(items []string) string {
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("  %s %s\n", Dim.Render(SymbolBullet), item))
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// NumberedList formats a slice of strings as a numbered list.
func NumberedList(items []string) string {
	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("  %s %s\n", Dim.Render(fmt.Sprintf("%d.", i+1)), item))
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
	running  bool
	frameIdx int
}

// SpinnerFrames are the animation frames for the spinner.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerFramesSimple are simpler ASCII frames for terminals without unicode.
var SpinnerFramesSimple = []string{"|", "/", "-", "\\"}

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

// Start begins the spinner animation.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.done = make(chan struct{})
	s.mu.Unlock()

	go func() {
		for {
			select {
			case <-s.done:
				return
			default:
				s.mu.Lock()
				msg := s.message
				frame := Info.Render(s.frames[s.frameIdx%len(s.frames)])
				fmt.Fprint(s.writer, ansiClearLine)
				fmt.Fprintf(s.writer, "%s %s", frame, msg)
				s.frameIdx++
				s.mu.Unlock()
				time.Sleep(s.interval)
			}
		}
	}()
}

// Stop stops the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.done)
	s.running = false

	fmt.Fprint(s.writer, ansiClearLine)
}

// StopWithMessage stops the spinner and prints a final message.
func (s *Spinner) StopWithMessage(status, message string) {
	s.Stop()
	fmt.Fprintf(s.writer, "%s %s\n", StatusIcon(status), message)
}

// UpdateMessage updates the spinner message.
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

type SpinnerOutputWriter struct {
	spinner *Spinner
	writer  io.Writer
}

func NewSpinnerOutputWriter(spinner *Spinner, writer io.Writer) io.Writer {
	return &SpinnerOutputWriter{
		spinner: spinner,
		writer:  writer,
	}
}

func (w *SpinnerOutputWriter) Write(p []byte) (int, error) {
	if w.spinner == nil {
		return w.writer.Write(p)
	}

	w.spinner.mu.Lock()
	defer w.spinner.mu.Unlock()

	if w.spinner.running {
		fmt.Fprint(w.spinner.writer, ansiClearLine)
	}

	n, err := w.writer.Write(p)
	if err != nil {
		return n, err
	}

	if w.spinner.running {
		spacing := "\n\n"
		if strings.HasSuffix(string(p), "\n") {
			spacing = "\n"
		}
		fmt.Fprint(w.spinner.writer, spacing)
		frame := Info.Render(w.spinner.frames[w.spinner.frameIdx%len(w.spinner.frames)])
		fmt.Fprint(w.spinner.writer, ansiClearLine)
		fmt.Fprintf(w.spinner.writer, "%s %s", frame, w.spinner.message)
		w.spinner.frameIdx++
	}

	return n, nil
}

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

func (p *ProgressBar) render() {
	if p.total <= 0 {
		return
	}

	percent := float64(p.current) / float64(p.total)
	filled := int(percent * float64(p.width))
	empty := p.width - filled

	bar := Success.Render(strings.Repeat("█", filled)) +
		Dim.Render(strings.Repeat("░", empty))

	pctStr := fmt.Sprintf("%3.0f%%", percent*100)
	sizeStr := fmt.Sprintf("%s / %s", FormatBytes(p.current), FormatBytes(p.total))

	fmt.Fprintf(p.writer, "\r%s %s %s %s",
		p.message,
		bar,
		Info.Render(pctStr),
		Dim.Render(sizeStr))
}

// Finish completes the progress bar.
func (p *ProgressBar) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = p.total
	p.render()
	fmt.Fprintln(p.writer)
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

// FormatDuration formats a duration in a human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// PrintSuccess prints a success message.
func PrintSuccess(message string) {
	fmt.Printf("%s %s\n", StatusIcon("success"), message)
}

// PrintWarning prints a warning message.
func PrintWarning(message string) {
	fmt.Printf("%s %s\n", StatusIcon("warning"), message)
}

// PrintError prints an error message.
func PrintError(message string) {
	fmt.Printf("%s %s\n", StatusIcon("error"), message)
}

// PrintInfo prints an info message.
func PrintInfo(message string) {
	fmt.Printf("%s %s\n", StatusIcon("info"), message)
}

// PrintStep prints a step message.
func PrintStep(current, total int, message string) {
	fmt.Println(StepWithLabel(current, total, message))
}

// PrintSubstep prints an indented substep message with arrow.
func PrintSubstep(message string) {
	fmt.Printf("%s%s %s\n", Indent2, Dim.Render(SymbolArrow), message)
}

// PrintDetail prints an indented detail message (dimmed).
func PrintDetail(message string) {
	fmt.Printf("%s%s\n", Indent2, Dim.Render(message))
}

// PrintKeyValue prints a key-value pair.
func PrintKeyValue(key, value string) {
	fmt.Println(KeyValue(key, value))
}

// SuccessBox prints a success message with optional key-value details.
// This replaces heavy double-line separators with a cleaner format.
func SuccessBox(message string, details []KVPair) {
	fmt.Printf("%s %s\n", StatusIcon("success"), SuccessBold.Render(message))
	if len(details) > 0 {
		fmt.Println()
		PrintKeyValuePadded(details)
	}
}

// InfoBox prints an info message with optional key-value details.
func InfoBox(message string, details []KVPair) {
	fmt.Printf("%s %s\n", StatusIcon("info"), Bold.Render(message))
	if len(details) > 0 {
		fmt.Println()
		PrintKeyValuePadded(details)
	}
}

// WarningBox prints a warning message with optional key-value details.
func WarningBox(message string, details []KVPair) {
	fmt.Printf("%s %s\n", StatusIcon("warning"), WarningBold.Render(message))
	if len(details) > 0 {
		fmt.Println()
		PrintKeyValuePadded(details)
	}
}

// ErrorBox prints an error message with optional key-value details.
func ErrorBox(message string, details []KVPair) {
	fmt.Printf("%s %s\n", StatusIcon("error"), ErrorBold.Render(message))
	if len(details) > 0 {
		fmt.Println()
		PrintKeyValuePadded(details)
	}
}

// Confirm displays a confirmation prompt and returns the result.
// Note: This is a simple blocking prompt. For TUI, use huh forms.
func Confirm(prompt string, defaultYes bool) bool {
	var response string
	defaultStr := "y/N"
	if defaultYes {
		defaultStr = "Y/n"
	}

	fmt.Printf("%s [%s]: ", prompt, defaultStr)
	fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "" {
		return defaultYes
	}
	return response == "y" || response == "yes"
}
