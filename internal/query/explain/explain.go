package explain

import (
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

// AnalysisSummary encapsulates human-readable takeaways from an execution plan
type AnalysisSummary struct {
	TotalCost       float64  `json:"total_cost"`
	TotalTimeMs     float64  `json:"total_time_ms"`
	HasFullScan     bool     `json:"has_full_scan"`
	FullScanTables  []string `json:"full_scan_tables"`
	ExpensiveNodes  []string `json:"expensive_nodes"`
	HumanSummary    string   `json:"human_summary"`
	FormattedTree   string   `json:"formatted_tree"`
}

// PlanAnalyzer analyzes execution plans
type PlanAnalyzer struct{}

func NewPlanAnalyzer() *PlanAnalyzer {
	return &PlanAnalyzer{}
}

func (p *PlanAnalyzer) Analyze(result *database.ExplainResult) *AnalysisSummary {
	if result == nil {
		return &AnalysisSummary{
			HumanSummary: "No execution plan available.",
		}
	}

	summary := &AnalysisSummary{
		TotalCost:      result.TotalCost,
		TotalTimeMs:    result.TotalTimeMs,
		HasFullScan:    result.HasFullScan,
		FullScanTables: result.FullScanTabs,
		FormattedTree:  ui.RenderPlanTree(result.RootNode),
	}

	// Traverse tree to collect insights
	var expensive []string
	if result.RootNode != nil {
		collectExpensiveNodes(result.RootNode, &expensive)
	}
	summary.ExpensiveNodes = expensive

	var explanations []string
	if result.HasFullScan {
		tabs := strings.Join(result.FullScanTabs, ", ")
		if tabs == "" {
			tabs = "the queried table"
		}
		explanations = append(explanations, fmt.Sprintf("• Full table scan on %s: The query engine must inspect every row in this table.", tabs))
	} else {
		explanations = append(explanations, "• Index scans active: The database is leveraging indexes to locate matching rows.")
	}

	if result.UsesFilesort {
		explanations = append(explanations, "• Sorting required: Rows are sorted after retrieval because no index satisfies the ORDER BY clause directly.")
	}
	if result.UsesTemp {
		explanations = append(explanations, "• Temporary table: The database created an intermediate structure to compute groupings or joins.")
	}

	if len(explanations) == 0 {
		explanations = append(explanations, "• Optimal plan: The query executed with efficient index lookups.")
	}

	summary.HumanSummary = strings.Join(explanations, "\n")
	return summary
}

func collectExpensiveNodes(node *database.PlanNode, list *[]string) {
	if node == nil {
		return
	}
	if strings.Contains(strings.ToUpper(node.ScanType), "ALL") || strings.Contains(strings.ToUpper(node.ScanType), "SEQ SCAN") {
		*list = append(*list, fmt.Sprintf("%s on %s (Full Scan)", node.Operation, node.Table))
	} else if node.Cost > 500.0 {
		*list = append(*list, fmt.Sprintf("%s on %s (Cost: %.1f)", node.Operation, node.Table, node.Cost))
	}

	for _, child := range node.Children {
		collectExpensiveNodes(child, list)
	}
}
