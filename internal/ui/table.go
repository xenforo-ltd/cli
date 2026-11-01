package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// tableStyle defines the visual style for tables.
type tableStyle struct {
	Header    lipgloss.Style
	Cell      lipgloss.Style
	Separator string
}

// tablePadding is the number of spaces to pad between columns.
const tablePadding = 2

// defaultTableStyle returns the default table styling.
func defaultTableStyle() tableStyle {
	return tableStyle{
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSubtle).
			PaddingRight(tablePadding),
		Cell: lipgloss.NewStyle().
			PaddingRight(tablePadding),
		Separator: "  ",
	}
}

// NewTable creates a formatted table with the default style.
func NewTable(headers []string, rows [][]string) string {
	style := defaultTableStyle()

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

// PrintTable prints a formatted table to stdout.
func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	lipgloss.Println(NewTable(headers, rows))
}
