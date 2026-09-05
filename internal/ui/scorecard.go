package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderScoreMeter renders a visual health/performance score meter (0-100)
func RenderScoreMeter(score int) string {
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	totalBlocks := 20
	filledBlocks := (score * totalBlocks) / 100

	var color lipgloss.Color
	var label string
	switch {
	case score >= 85:
		color = SuccessColor
		label = "HEALTHY / OPTIMAL"
	case score >= 60:
		color = WarningColor
		label = "NEEDS ATTENTION"
	default:
		color = DangerColor
		label = "CRITICAL ISSUES DETECTED"
	}

	meter := strings.Repeat("█", filledBlocks) + strings.Repeat("░", totalBlocks-filledBlocks)
	styledMeter := lipgloss.NewStyle().Foreground(color).Bold(true).Render(meter)
	styledScore := lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%3d / 100", score))
	styledLabel := lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("[%s]", label))

	return fmt.Sprintf("[%s] %s  %s", styledMeter, styledScore, styledLabel)
}

// RenderCard renders content inside a styled bordered card with a title
func RenderCard(title string, content string) string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(PrimaryColor).
		Render(fmt.Sprintf("╭─ %s ─%s╮", title, strings.Repeat("─", max(0, 60-len(title)))))

	lines := strings.Split(content, "\n")
	var body []string
	for _, l := range lines {
		body = append(body, fmt.Sprintf("│ %-62s │", l))
	}
	footer := lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Render(fmt.Sprintf("╰%s╯", strings.Repeat("─", 65)))

	return fmt.Sprintf("%s\n%s\n%s", header, strings.Join(body, "\n"), footer)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
