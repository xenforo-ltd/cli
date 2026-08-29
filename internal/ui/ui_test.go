package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func TestPlural(t *testing.T) {
	if got := Plural(1, "license", "licenses"); got != "1 license" {
		t.Errorf("got %q", got)
	}
	if got := Plural(3, "license", "licenses"); got != "3 licenses" {
		t.Errorf("got %q", got)
	}
	if got := Plural(0, "entry", "entries"); got != "0 entries" {
		t.Errorf("got %q", got)
	}
}

func TestFormatDates(t *testing.T) {
	ts := time.Date(2026, 8, 18, 14, 5, 9, 0, time.UTC)
	if got := FormatDate(ts); got != "2026-08-18" {
		t.Errorf("got %q", got)
	}
	if got := FormatDateTime(ts); got != "2026-08-18 14:05" {
		t.Errorf("got %q", got)
	}
}

func TestKeyValuePaddingUsesDisplayWidth(t *testing.T) {
	// Styled keys must not break alignment: pad by lipgloss.Width, not len.
	pairs := []KVPair{KV("Café", "a"), KV("Longer key", "b")}
	// renderKeyValuePadded is the new testable core returning a string.
	out := renderKeyValuePadded(pairs, Indent1)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	line0 := stripANSI(lines[0])
	line1 := stripANSI(lines[1])
	// Compare column (rune) position, not byte offset: byte length and
	// display width diverge for multi-byte runes (e.g. "é" is 2 bytes but
	// 1 column), while display-width padding aligns columns, not bytes.
	col0 := utf8.RuneCountInString(line0[:strings.Index(line0, " a")])
	col1 := utf8.RuneCountInString(line1[:strings.Index(line1, " b")])
	if col0 != col1 {
		t.Errorf("values misaligned:\n%s", out)
	}
}

func TestStatusIconSymbols(t *testing.T) {
	if !strings.Contains(StatusIcon("success"), SymbolSuccess) {
		t.Fatal("success icon missing symbol")
	}

	if !strings.Contains(StatusIcon("warning"), SymbolWarning) {
		t.Fatal("warning icon missing symbol")
	}

	if !strings.Contains(StatusIcon("error"), SymbolError) {
		t.Fatal("error icon missing symbol")
	}

	if !strings.Contains(StatusIcon("unknown"), "?") {
		t.Fatal("unknown icon should include ?")
	}
}

func TestStepAndIndentHelpers(t *testing.T) {
	if got := StepWithLabel(1, 3, "Init"); !strings.Contains(got, "Init") || !strings.Contains(got, "1/3") {
		t.Fatalf("unexpected StepWithLabel output: %q", got)
	}

	indented := Indent("a\n\nb", 2)
	if indented != "  a\n\n  b" {
		t.Fatalf("Indent output mismatch: %q", indented)
	}

	lines := IndentLines([]string{"x", "", "y"}, 3)
	if lines[0] != "   x" || lines[1] != "" || lines[2] != "   y" {
		t.Fatalf("IndentLines output mismatch: %#v", lines)
	}
}

func TestListFormatting(t *testing.T) {
	list := List([]string{"one", "two"})
	if !strings.Contains(list, "one") || !strings.Contains(list, "two") {
		t.Fatalf("List output mismatch: %q", list)
	}
}

// withTTY forces the package's TTY detection for the duration of a test, so
// spinner and progress-bar behaviour can be exercised deterministically.
func withTTY(t *testing.T, on bool) {
	t.Helper()

	previous := isTTY
	isTTY = on

	t.Cleanup(func() { isTTY = previous })
}

func TestShortHomeAbbreviatesHomePrefix(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory available")
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"home itself", home, "~"},
		{"path under home", filepath.Join(home, "Sites", "main"), filepath.Join("~", "Sites", "main")},
		{"unrelated path", "/var/tmp/thing", "/var/tmp/thing"},
		{"prefix but not a child", home + "-other", home + "-other"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShortHome(tc.in); got != tc.want {
				t.Errorf("ShortHome(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEnabledRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	if Enabled(os.Stdout) {
		t.Error("NO_COLOR is set, so styling must be disabled")
	}

	t.Setenv("NO_COLOR", "")

	// Without NO_COLOR the answer follows TTY detection, which is false when
	// tests capture output, so this only asserts NO_COLOR is not the decider.
	if Enabled(os.Stdout) != IsTerminal(os.Stdout) {
		t.Error("without NO_COLOR, Enabled must follow IsTerminal")
	}
}

func TestSpinnerWithoutTTYPrintsMessageOnceAndDoesNotAnimate(t *testing.T) {
	withTTY(t, false)

	var buf bytes.Buffer

	s := NewSpinner("Working")
	s.writer = &buf
	s.Start()
	s.Stop()

	if got := buf.String(); got != "Working\n" {
		t.Errorf("non-TTY spinner wrote %q, want %q", got, "Working\n")
	}

	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("non-TTY spinner emitted ANSI escapes: %q", buf.String())
	}
}

func TestSpinnerStopWithMessagePrintsFinalLine(t *testing.T) {
	withTTY(t, false)

	var buf bytes.Buffer

	s := NewSpinner("Downloading")
	s.writer = &buf
	s.Start()
	s.StopWithMessage("success", "Downloaded")

	out := stripANSI(buf.String())
	if !strings.Contains(out, "Downloaded") {
		t.Errorf("final message missing from %q", out)
	}

	if !strings.Contains(out, SymbolSuccess) {
		t.Errorf("success icon missing from %q", out)
	}
}

func TestSpinnerStopIsIdempotentAndWaitsForTheAnimation(t *testing.T) {
	withTTY(t, true)

	var buf bytes.Buffer

	s := NewSpinner("Working")
	s.writer = &buf
	s.interval = time.Millisecond

	s.Start()
	time.Sleep(5 * time.Millisecond)
	s.Stop()

	// A second Stop must not panic on the already-closed channel, and a
	// restart must be safe now that Stop waits for the goroutine to exit.
	s.Stop()
	s.Start()
	s.Stop()
}

func TestSpinnerUpdateMessageChangesTheRenderedLine(t *testing.T) {
	withTTY(t, true)

	var buf bytes.Buffer

	s := NewSpinner("First")
	s.writer = &buf
	s.interval = time.Millisecond

	s.Start()
	time.Sleep(5 * time.Millisecond)
	s.UpdateMessage("Second")
	time.Sleep(5 * time.Millisecond)
	s.Stop()

	if !strings.Contains(stripANSI(buf.String()), "Second") {
		t.Errorf("updated message never rendered: %q", stripANSI(buf.String()))
	}
}

func TestSpinnerOutputWriterPassesThroughWithoutASpinner(t *testing.T) {
	var buf bytes.Buffer

	w := NewSpinnerOutputWriter(nil, &buf)

	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned %v", err)
	}

	if n != 5 || buf.String() != "hello" {
		t.Errorf("wrote %d bytes %q, want 5 %q", n, buf.String(), "hello")
	}
}

func TestSpinnerOutputWriterDoesNotDoubleSpaceStreamedOutput(t *testing.T) {
	withTTY(t, true)

	var spinnerBuf, outBuf bytes.Buffer

	s := NewSpinner("Working")
	s.writer = &spinnerBuf
	s.interval = time.Hour // no animation frames during the test

	s.Start()

	w := NewSpinnerOutputWriter(s, &outBuf)
	if _, err := w.Write([]byte("chunk without newline")); err != nil {
		t.Fatalf("Write returned %v", err)
	}

	s.Stop()

	if strings.Contains(spinnerBuf.String(), "\n\n") {
		t.Errorf("spinner repaint inserted a blank line: %q", spinnerBuf.String())
	}

	if outBuf.String() != "chunk without newline" {
		t.Errorf("payload altered: %q", outBuf.String())
	}
}

func TestProgressBarRendersProgressAndFinishes(t *testing.T) {
	withTTY(t, true)

	var buf bytes.Buffer

	p := NewProgressBar(100, "asset.tar.gz")
	p.writer = &buf

	p.Update(50)

	mid := stripANSI(buf.String())
	if !strings.Contains(mid, "50%") {
		t.Errorf("expected 50%% in %q", mid)
	}

	if !strings.Contains(mid, "asset.tar.gz") {
		t.Errorf("expected the label in %q", mid)
	}

	p.Finish()

	if !strings.Contains(stripANSI(buf.String()), "100%") {
		t.Errorf("Finish did not render 100%%: %q", stripANSI(buf.String()))
	}
}

func TestProgressBarIncrementClampsToTotal(t *testing.T) {
	withTTY(t, true)

	var buf bytes.Buffer

	p := NewProgressBar(10, "")
	p.writer = &buf

	p.Increment(99)

	if p.current != 10 {
		t.Errorf("current = %d, want it clamped to 10", p.current)
	}
}

func TestProgressBarAbandonDoesNotReportCompletion(t *testing.T) {
	withTTY(t, true)

	var buf bytes.Buffer

	p := NewProgressBar(100, "asset.tar.gz")
	p.writer = &buf

	p.Update(25)
	buf.Reset()
	p.Abandon()

	if strings.Contains(stripANSI(buf.String()), "100%") {
		t.Errorf("Abandon reported the transfer as complete: %q", stripANSI(buf.String()))
	}

	if p.current != 25 {
		t.Errorf("Abandon changed progress to %d, want it left at 25", p.current)
	}
}

func TestProgressBarWithoutTTYWritesNothing(t *testing.T) {
	withTTY(t, false)

	var buf bytes.Buffer

	p := NewProgressBar(100, "asset.tar.gz")
	p.writer = &buf

	p.Update(50)
	p.Finish()

	if buf.Len() != 0 {
		t.Errorf("non-TTY progress bar wrote %q, want nothing", buf.String())
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
	}

	for _, tc := range cases {
		if got := FormatBytes(tc.in); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
