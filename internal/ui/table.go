package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"
)

// Table renders formatted tables in the terminal
type Table struct {
	headers   []string
	rows      [][]string
	truncated bool
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

func (t *Table) WasTruncated() bool {
	return t.truncated
}

func getTerminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 {
		return w
	}
	if w, _, err := term.GetSize(int(os.Stdin.Fd())); err == nil && w > 0 {
		return w
	}
	return 0
}

func (t *Table) Render() string {
	if len(t.headers) == 0 && len(t.rows) == 0 {
		return ""
	}

	numCols := len(t.headers)
	for _, row := range t.rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}

	// Normalize headers
	headers := make([]string, numCols)
	for i := 0; i < numCols; i++ {
		if i < len(t.headers) {
			headers[i] = t.headers[i]
		} else {
			headers[i] = ""
		}
	}

	// Normalize rows
	normalizedRows := make([][]string, len(t.rows))
	for rIdx, row := range t.rows {
		r := make([]string, numCols)
		for cIdx := 0; cIdx < numCols; cIdx++ {
			if cIdx < len(row) {
				r[cIdx] = strings.ReplaceAll(row[cIdx], "\r", "")
			} else {
				r[cIdx] = ""
			}
		}
		normalizedRows[rIdx] = r
	}

	tbl := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(BorderColor)).
		Headers(headers...).
		Rows(normalizedRows...).
		Wrap(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().
					Bold(true).
					Foreground(SecondaryColor).
					Padding(0, 1)
			}
			s := lipgloss.NewStyle().Padding(0, 1)
			if row%2 == 1 {
				s = s.Foreground(lipgloss.Color("#E2E8F0"))
			} else {
				s = s.Foreground(lipgloss.Color("#FFFFFF"))
			}
			return s
		})

	termWidth := getTerminalWidth()
	if termWidth > 40 {
		// Calculate natural width of the table
		naturalWidth := 1 // left border
		for cIdx := 0; cIdx < numCols; cIdx++ {
			maxW := lipgloss.Width(headers[cIdx])
			for _, r := range normalizedRows {
				w := lipgloss.Width(r[cIdx])
				if w > maxW {
					maxW = w
				}
			}
			naturalWidth += maxW + 2 + 1 // padding(2) + border(1)
		}

		if naturalWidth > termWidth {
			tbl.Width(termWidth)
			t.truncated = true
		}
	}

	return tbl.Render()
}
