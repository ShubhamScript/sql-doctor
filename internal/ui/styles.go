package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Theme Colors
	PrimaryColor   = lipgloss.Color("#6366F1") // Indigo
	SecondaryColor = lipgloss.Color("#38BDF8") // Sky Blue
	SuccessColor   = lipgloss.Color("#10B981") // Emerald Green
	WarningColor   = lipgloss.Color("#F59E0B") // Amber
	DangerColor    = lipgloss.Color("#EF4444") // Red
	MutedColor     = lipgloss.Color("#6B7280") // Gray
	BgDark         = lipgloss.Color("#1E1E2E") // Midnight
	BorderColor    = lipgloss.Color("#4B5563")

	// Text Styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			MarginBottom(1)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(SecondaryColor)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(MutedColor).
			Italic(true)

	// Box Styles
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(PrimaryColor).
			Padding(1, 2).
			MarginBottom(1)

	AlertBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(WarningColor).
			Padding(0, 1)

	// Badges
	CriticalBadge = lipgloss.NewStyle().
			Bold(true).
			Background(DangerColor).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render("CRITICAL")

	WarningBadge = lipgloss.NewStyle().
			Bold(true).
			Background(WarningColor).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1).
			Render("WARNING")

	InfoBadge = lipgloss.NewStyle().
			Bold(true).
			Background(SecondaryColor).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1).
			Render("INFO")

	SuccessBadge = lipgloss.NewStyle().
			Bold(true).
			Background(SuccessColor).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render("OPTIMAL")

	// Table Headers
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(SecondaryColor).
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(BorderColor)

	TableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)
)

// Banner renders the SQL Doctor banner
func Banner() string {
	title := TitleStyle.Render("SQL DOCTOR — Database Diagnostics & SQL Intelligence Toolkit")
	bar := lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("─", 65))
	return fmt.Sprintf("%s\n%s", title, bar)
}

// Success renders a formatted success message
func Success(format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	return lipgloss.NewStyle().Foreground(SuccessColor).Bold(true).Render("✓ " + msg)
}

// Warning renders a formatted warning message
func Warning(format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	return lipgloss.NewStyle().Foreground(WarningColor).Bold(true).Render("⚠ " + msg)
}

// Error renders a formatted error message
func Error(format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	return lipgloss.NewStyle().Foreground(DangerColor).Bold(true).Render("✗ " + msg)
}

// Info renders a formatted info message
func Info(format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	return lipgloss.NewStyle().Foreground(SecondaryColor).Render("ℹ " + msg)
}
