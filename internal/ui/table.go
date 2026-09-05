package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table renders formatted tables in the terminal
type Table struct {
	headers []string
	rows    [][]string
}

func NewTable(headers ...string) *Table {
	return &Table{
		headers: headers,
	}
}

func (t *Table) AddRow(cols ...string) *Table {
	t.rows = append(t.rows, cols)
	return t
}

func (t *Table) Render() string {
	if len(t.headers) == 0 && len(t.rows) == 0 {
		return ""
	}

	colWidths := make([]int, len(t.headers))
	for i, h := range t.headers {
		colWidths[i] = lipgloss.Width(h)
	}

	for _, row := range t.rows {
		for i, col := range row {
			if i < len(colWidths) {
				w := lipgloss.Width(col)
				if w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	var sb strings.Builder

	// Header line
	var headerCells []string
	for i, h := range t.headers {
		cell := lipgloss.NewStyle().
			Width(colWidths[i] + 2).
			Bold(true).
			Foreground(SecondaryColor).
			Render(h)
		headerCells = append(headerCells, cell)
	}
	sb.WriteString(strings.Join(headerCells, "│"))
	sb.WriteString("\n")

	// Separator
	var sepCells []string
	for _, w := range colWidths {
		sepCells = append(sepCells, strings.Repeat("─", w+2))
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Join(sepCells, "┼")))
	sb.WriteString("\n")

	// Data rows
	for rowIdx, row := range t.rows {
		var rowCells []string
		for i := 0; i < len(t.headers); i++ {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			style := lipgloss.NewStyle().Width(colWidths[i] + 2)
			if rowIdx%2 == 1 {
				style = style.Foreground(lipgloss.Color("#E2E8F0"))
			}
			rowCells = append(rowCells, style.Render(val))
		}
		sb.WriteString(strings.Join(rowCells, "│"))
		sb.WriteString("\n")
	}

	return sb.String()
}
