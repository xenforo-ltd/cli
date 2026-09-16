package ui

import (
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
