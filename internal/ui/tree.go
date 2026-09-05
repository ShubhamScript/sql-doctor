package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sql-doctor/sql-doctor/internal/database"
)

// RenderPlanTree formats a database.PlanNode into an ASCII/Unicode hierarchy tree
func RenderPlanTree(root *database.PlanNode) string {
	if root == nil {
		return ""
	}
	var sb strings.Builder
	renderNode(root, "", true, &sb)
	return sb.String()
}

func renderNode(node *database.PlanNode, prefix string, isLast bool, sb *strings.Builder) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	label := node.Operation
	if node.Table != "" {
		label += fmt.Sprintf(" on %s", lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true).Render(node.Table))
	}
	if node.Index != "" {
		label += fmt.Sprintf(" using index %s", lipgloss.NewStyle().Foreground(PrimaryColor).Render(node.Index))
	}

	details := []string{}
	if node.ScanType != "" {
		scanStyle := lipgloss.NewStyle()
		if strings.Contains(strings.ToUpper(node.ScanType), "ALL") || strings.Contains(strings.ToUpper(node.ScanType), "SEQ SCAN") {
			scanStyle = scanStyle.Foreground(DangerColor).Bold(true)
		} else {
			scanStyle = scanStyle.Foreground(SuccessColor)
		}
		details = append(details, scanStyle.Render(node.ScanType))
	}
	if node.Cost > 0 {
		details = append(details, fmt.Sprintf("cost: %.2f", node.Cost))
	}
	if node.EstRows > 0 {
		details = append(details, fmt.Sprintf("est_rows: %.0f", node.EstRows))
	}
	if node.ActualRows > 0 {
		details = append(details, fmt.Sprintf("act_rows: %.0f", node.ActualRows))
	}
	if node.ActualTimeMs > 0 {
		details = append(details, fmt.Sprintf("time: %.2fms", node.ActualTimeMs))
	}

	if len(details) > 0 {
		label += fmt.Sprintf(" (%s)", strings.Join(details, ", "))
	}

	sb.WriteString(prefix)
	sb.WriteString(connector)
	sb.WriteString(label)
	sb.WriteByte('\n')

	newPrefix := prefix + "│   "
	if isLast {
		newPrefix = prefix + "    "
	}

	for i, child := range node.Children {
		isChildLast := i == len(node.Children)-1
		renderNode(child, newPrefix, isChildLast, sb)
	}
}
