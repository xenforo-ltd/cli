package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// TableStyle defines the visual style for tables.
type TableStyle struct {
	Header    lipgloss.Style
	Cell      lipgloss.Style
	Separator string
}

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

func NewTable(headers []string, rows [][]string) string {
	style := DefaultTableStyle()

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return style.Header
			}
			return style.Cell
		})

	return t.String()
}

// The styleFunc receives (row, col) where row == -1 indicates the header.
func NewTableWithStyles(headers []string, rows [][]string, styleFunc func(row, col int) lipgloss.Style) string {
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(styleFunc)

	return t.String()
}

func StatusTableStyle(statusCol int, statusMap map[string]lipgloss.Style) func(row, col int) lipgloss.Style {
	defaultStyle := DefaultTableStyle()

	return func(row, col int) lipgloss.Style {
		if row == table.HeaderRow {
			return defaultStyle.Header
		}
		return defaultStyle.Cell
	}
}

func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	println(NewTable(headers, rows))
}
