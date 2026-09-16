package ui

import (
	"strings"
	"testing"
)

func TestDefaultTableStyle(t *testing.T) {
	s := DefaultTableStyle()
	if s.Separator != "  " {
		t.Fatalf("separator = %q", s.Separator)
	}
}

func TestNewTableFunctions(t *testing.T) {
	headers := []string{"A", "B"}
	rows := [][]string{{"1", "2"}}

	out := NewTable(headers, rows)
	if !strings.Contains(out, "A") || !strings.Contains(out, "1") {
		t.Fatalf("unexpected table output: %q", out)
	}
}
