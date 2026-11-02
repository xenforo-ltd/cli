package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// tableStyle defines the visual style for tables.
type tableStyle struct {
	header lipgloss.Style
	cell   lipgloss.Style
}

// tablePadding is the number of spaces to pad between columns.
const tablePadding = 2

// defaultTableStyle returns the default table styling.
func defaultTableStyle() tableStyle {
	return tableStyle{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSubtle).
			PaddingRight(tablePadding),
		cell: lipgloss.NewStyle().
			PaddingRight(tablePadding),
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
				return style.header
			}

			return style.cell
		})

	return t.String()
}
