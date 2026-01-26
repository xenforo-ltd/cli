package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

	custom := NewTableWithStyles(headers, rows, func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle()
	})
	if !strings.Contains(custom, "B") || !strings.Contains(custom, "2") {
		t.Fatalf("unexpected custom table output: %q", custom)
	}
}

func TestStatusTableStyle(t *testing.T) {
	fn := StatusTableStyle(1, map[string]lipgloss.Style{})
	if fn == nil {
		t.Fatal("expected style func")
	}
}
