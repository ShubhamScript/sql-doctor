package optimizer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/query/parser"
)

// IndexRecommendation details a proposed index
type IndexRecommendation struct {
	TableName      string   `json:"table_name"`
	IndexName      string   `json:"index_name"`
	Columns        []string `json:"columns"`
	EstimatedGain  string   `json:"estimated_gain"` // HIGH, MEDIUM, LOW
	SuggestedDDL   string   `json:"suggested_ddl"`
	Rationale      string   `json:"rationale"`
	Tradeoffs      string   `json:"tradeoffs"`
}

// RewriteSuggestion details an alternative query structure
type RewriteSuggestion struct {
	OriginalSnippet string `json:"original_snippet"`
	SuggestedRewrite string `json:"suggested_rewrite"`
	Rationale        string `json:"rationale"`
}

// OptimizationResult aggregates all optimization suggestions
type OptimizationResult struct {
	QueryText            string                `json:"query_text"`
	IndexRecommendations []IndexRecommendation `json:"index_recommendations"`
	RewriteSuggestions   []RewriteSuggestion   `json:"rewrite_suggestions"`
	Summary              string                `json:"summary"`
}

// Optimizer performs deterministic index and query optimization
type Optimizer struct {
	driver database.Driver
	parser parser.Parser
}

func NewOptimizer(driver database.Driver) *Optimizer {
	return &Optimizer{
		driver: driver,
		parser: parser.NewVitessParser(),
	}
}

func (o *Optimizer) Optimize(ctx context.Context, db *sql.DB, query string) (*OptimizationResult, error) {
	parsed, err := o.parser.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query for optimization: %w", err)
	}

	result := &OptimizationResult{
		QueryText: query,
	}

	// 1. Generate Index Recommendations
	tableCols := make(map[string]map[string]string) // table -> col -> op
	for _, pred := range parsed.Predicates {
		tbl := pred.Table
		if tbl == "" && len(parsed.Tables) == 1 {
			tbl = parsed.Tables[0]
		}
		if tbl == "" {
			continue
		}
		if _, ok := tableCols[tbl]; !ok {
			tableCols[tbl] = make(map[string]string)
		}
		tableCols[tbl][pred.Column] = pred.Operator
	}

	for tbl, cols := range tableCols {
		var equalityCols []string
		var rangeCols []string

		for col, op := range cols {
			if op == "=" || op == "<=>" {
				equalityCols = append(equalityCols, col)
			} else {
				rangeCols = append(rangeCols, col)
			}
		}

		// Add ORDER BY columns
		var sortCols []string
		for _, col := range parsed.OrderByCols {
			// check if not already in equality
			found := false
			for _, ec := range equalityCols {
				if ec == col {
					found = true
					break
				}
			}
			if !found {
				sortCols = append(sortCols, col)
			}
		}

		// Composite index order: Equality -> Range -> Sort
		var proposedCols []string
		proposedCols = append(proposedCols, equalityCols...)
		proposedCols = append(proposedCols, rangeCols...)
		for _, sc := range sortCols {
			alreadyIn := false
			for _, pc := range proposedCols {
				if pc == sc {
					alreadyIn = true
					break
				}
			}
			if !alreadyIn {
				proposedCols = append(proposedCols, sc)
			}
		}

		if len(proposedCols) > 0 {
			// Check if table already has an index covering this
			alreadyCovered := false
			if db != nil && o.driver != nil {
				existingIndexes, _ := o.driver.Indexes(ctx, db, tbl)
				for _, idx := range existingIndexes {
					if len(idx.Columns) >= len(proposedCols) {
						match := true
						for i, pc := range proposedCols {
							if i >= len(idx.Columns) || !strings.EqualFold(idx.Columns[i], pc) {
								match = false
								break
							}
						}
						if match {
							alreadyCovered = true
							break
						}
					}
				}
			}

			if !alreadyCovered {
				idxName := fmt.Sprintf("idx_%s_%s", tbl, strings.Join(proposedCols, "_"))
				if len(idxName) > 60 {
					idxName = idxName[:60]
				}

				ddl := fmt.Sprintf("CREATE INDEX %s ON %s (%s);", idxName, tbl, strings.Join(proposedCols, ", "))
				result.IndexRecommendations = append(result.IndexRecommendations, IndexRecommendation{
					TableName:     tbl,
					IndexName:     idxName,
					Columns:       proposedCols,
					EstimatedGain: "HIGH",
					SuggestedDDL:  ddl,
					Rationale:     fmt.Sprintf("Composite index ordering follows the Equality-Range-Sort heuristic for columns (%s). Eliminates full table scans.", strings.Join(proposedCols, ", ")),
					Tradeoffs:     "Increases disk footprint and slightly adds insert/update write latency on the table.",
				})
			}
		}
	}

	// 2. Query Rewrites
	if parsed.HasSelectAll {
		result.RewriteSuggestions = append(result.RewriteSuggestions, RewriteSuggestion{
			OriginalSnippet: "SELECT *",
			SuggestedRewrite: "SELECT id, [explicit_columns...]",
			Rationale:        "Enables index-only covering scans and avoids scanning unnecessary wide columns.",
		})
	}

	for _, pred := range parsed.Predicates {
		if pred.HasFunction {
			result.RewriteSuggestions = append(result.RewriteSuggestions, RewriteSuggestion{
				OriginalSnippet: fmt.Sprintf("FUNC(%s)", pred.Column),
				SuggestedRewrite: fmt.Sprintf("%s >= ? AND %s < ?", pred.Column, pred.Column),
				Rationale:        "Unwrapping columns from SQL functions allows the query planner to perform direct B-Tree index range scans.",
			})
		}
	}

	if len(result.IndexRecommendations) == 0 && len(result.RewriteSuggestions) == 0 {
		result.Summary = "No urgent indexing or query rewrite recommendations needed."
	} else {
		result.Summary = fmt.Sprintf("Identified %d index candidate(s) and %d rewrite suggestion(s).", len(result.IndexRecommendations), len(result.RewriteSuggestions))
	}

	return result, nil
}
