package ui

import (
	"strings"
	"testing"
)

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

	numbered := NumberedList([]string{"first", "second"})
	if !strings.Contains(numbered, "1.") || !strings.Contains(numbered, "2.") {
		t.Fatalf("NumberedList output mismatch: %q", numbered)
	}
}
