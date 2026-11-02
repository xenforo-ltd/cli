package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// tablePadding is the number of spaces to pad between columns.
const tablePadding = 2

// NewTable creates a formatted table with the default style.
func NewTable(headers []string, rows [][]string) string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorSubtle).
		PaddingRight(tablePadding)
	cellStyle := lipgloss.NewStyle().
		PaddingRight(tablePadding)

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}

			return cellStyle
		})

	return t.String()
}
