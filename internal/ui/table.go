package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// TableStyle defines the visual style for tables.
type TableStyle struct {
	Header    lipgloss.Style
	Cell      lipgloss.Style
	Separator string
}

// DefaultTableStyle returns the default table styling.
func DefaultTableStyle() TableStyle {
	return TableStyle{
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSubtle).
			PaddingRight(2),
		Cell: lipgloss.NewStyle().
			PaddingRight(2),
		Separator: "  ",
	}
}

// NewTable creates a formatted table with the default style.
func NewTable(headers []string, rows [][]string) string {
	style := DefaultTableStyle()

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return style.Header
			}

			return style.Cell
		})

	return t.String()
}

// NewTableWithStyles creates a table with styled cells.
// The styleFunc receives (row, col) where row == -1 indicates the header.
func NewTableWithStyles(headers []string, rows [][]string, styleFunc func(row, col int) lipgloss.Style) string {
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(styleFunc)

	return t.String()
}

// StatusTableStyle returns a style function for status-based table coloring.
func StatusTableStyle(_ int, _ map[string]lipgloss.Style) func(row, col int) lipgloss.Style {
	defaultStyle := DefaultTableStyle()

	return func(row, _ int) lipgloss.Style {
		if row == table.HeaderRow {
			return defaultStyle.Header
		}

		return defaultStyle.Cell
	}
}

// PrintTable prints a formatted table to stdout.
func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	lipgloss.Println(NewTable(headers, rows))
}
