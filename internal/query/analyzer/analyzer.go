package analyzer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sql-doctor/sql-doctor/internal/database"
	"github.com/sql-doctor/sql-doctor/internal/query/parser"
)

// MetricFinding describes an observed metric or diagnosis
type MetricFinding struct {
	Level       string `json:"level"` // CRITICAL, WARNING, INFO, OPTIMAL
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Remedy      string `json:"remedy"`
}

// QueryAnalysisResult encapsulates complete performance profile of a query
type QueryAnalysisResult struct {
	QueryText        string           `json:"query_text"`
	Fingerprint      string           `json:"fingerprint"`
	ExecutionTimeMs  float64          `json:"execution_time_ms"`
	RowsExamined     int64            `json:"rows_examined"`
	RowsReturned     int64            `json:"rows_returned"`
	ExaminedRatio    float64          `json:"examined_to_returned_ratio"`
	PerformanceScore int              `json:"performance_score"`
	HasFullTableScan bool             `json:"has_full_table_scan"`
	FullScanTables   []string         `json:"full_scan_tables,omitempty"`
	UsesFilesort     bool             `json:"uses_filesort"`
	UsesTempTable    bool             `json:"uses_temporary_table"`
	ExplainResult    *database.ExplainResult `json:"explain_result,omitempty"`
	LintFindings     []parser.LintFinding    `json:"lint_findings,omitempty"`
	Findings         []MetricFinding         `json:"findings"`
}

// Analyzer inspects and executes queries to analyze runtime behavior
type Analyzer struct {
	driver database.Driver
	parser parser.Parser
	linter *parser.Linter
}

func NewAnalyzer(driver database.Driver) *Analyzer {
	p := parser.NewVitessParser()
	return &Analyzer{
		driver: driver,
		parser: p,
		linter: parser.NewLinter(p),
	}
}

// Analyze executes EXPLAIN and (if read-only/safe) measures execution metrics
func (a *Analyzer) Analyze(ctx context.Context, db *sql.DB, query string) (*QueryAnalysisResult, error) {
	parsed, _ := a.parser.Parse(query)
	lintFindings, _ := a.linter.Lint(query)

	result := &QueryAnalysisResult{
		QueryText:    query,
		Fingerprint:  a.parser.Fingerprint(query),
		LintFindings: lintFindings,
	}

	// 1. Run EXPLAIN
	explainRes, err := a.driver.Explain(ctx, db, query, false)
	if err == nil && explainRes != nil {
		result.ExplainResult = explainRes
		result.HasFullTableScan = explainRes.HasFullScan
		result.FullScanTables = explainRes.FullScanTabs
		result.UsesFilesort = explainRes.UsesFilesort
		result.UsesTempTable = explainRes.UsesTemp
		result.RowsExamined = explainRes.RowsExamined
	}

	// 2. Measure actual query execution time if SELECT
	isSelect := parsed != nil && parsed.Type == parser.StmtSelect
	if isSelect {
		start := time.Now()
		rows, qErr := db.QueryContext(ctx, query)
		if qErr == nil {
			var count int64
			for rows.Next() {
				count++
			}
			rows.Close()
			result.ExecutionTimeMs = float64(time.Since(start).Microseconds()) / 1000.0
			result.RowsReturned = count
		}
	}

	if result.RowsReturned > 0 && result.RowsExamined > 0 {
		result.ExaminedRatio = float64(result.RowsExamined) / float64(result.RowsReturned)
	}

	// 3. Compute deterministic score and diagnose
	a.diagnoseAndScore(result)

	return result, nil
}

func (a *Analyzer) diagnoseAndScore(res *QueryAnalysisResult) {
	score := 100

	// Full table scan penalty
	if res.HasFullTableScan {
		score -= 35
		tabs := strings.Join(res.FullScanTables, ", ")
		if tabs == "" {
			tabs = "one or more tables"
		}
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "CRITICAL",
			Title:       "Full Table Scan (ALL)",
			Description: fmt.Sprintf("Query performs an unindexed full table scan on %s.", tabs),
			Impact:      "High disk I/O, cache thrashing, and high latency as table volume expands.",
			Remedy:      "Create an index on filter columns in the WHERE or JOIN clauses.",
		})
	}

	// Examined vs Returned ratio
	if res.ExaminedRatio > 50.0 {
		score -= 20
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "WARNING",
			Title:       "High Row Examination Ratio",
			Description: fmt.Sprintf("Query examined %.1fx more rows (%d) than it returned (%d).", res.ExaminedRatio, res.RowsExamined, res.RowsReturned),
			Impact:      "Wasted CPU cycles scanning and filtering non-matching records in memory.",
			Remedy:      "Add more selective index columns to avoid reading unneeded rows.",
		})
	}

	// Filesort penalty
	if res.UsesFilesort {
		score -= 15
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "WARNING",
			Title:       "Filesort / In-Memory Sorting Operation",
			Description: "Database engine had to perform an external or in-memory sort pass rather than utilizing index order.",
			Impact:      "Increased memory usage and CPU latency during ORDER BY or GROUP BY execution.",
			Remedy:      "Include ORDER BY columns in a composite index matching filter order.",
		})
	}

	// Temporary table penalty
	if res.UsesTempTable {
		score -= 15
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "WARNING",
			Title:       "Temporary Table Created",
			Description: "Query required creating an intermediate temporary table on disk or in memory.",
			Impact:      "Additional I/O overhead and memory allocation.",
			Remedy:      "Align GROUP BY and ORDER BY clauses, or optimize JOIN ordering.",
		})
	}

	// Execution time penalty
	if res.ExecutionTimeMs > 1000.0 {
		score -= 25
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "CRITICAL",
			Title:       "Slow Execution Time (> 1.0s)",
			Description: fmt.Sprintf("Query execution time was %.2fms, exceeding recommended interactive thresholds.", res.ExecutionTimeMs),
			Impact:      "Degraded end-user response times and potential connection starvation.",
			Remedy:      "Apply recommended indexes, restrict SELECT columns, and review query plan.",
		})
	} else if res.ExecutionTimeMs > 200.0 {
		score -= 10
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "WARNING",
			Title:       "Moderate Execution Time (> 200ms)",
			Description: fmt.Sprintf("Query executed in %.2fms.", res.ExecutionTimeMs),
			Impact:      "May create bottlenecks under high concurrent load.",
			Remedy:      "Review indexing and ensure pagination limits are enforced.",
		})
	}

	// Deduct for critical lint findings
	for _, l := range res.LintFindings {
		if l.Severity == parser.SeverityCritical {
			score -= 10
		} else if l.Severity == parser.SeverityWarning {
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	res.PerformanceScore = score

	if len(res.Findings) == 0 {
		res.Findings = append(res.Findings, MetricFinding{
			Level:       "OPTIMAL",
			Title:       "No Major Bottlenecks Detected",
			Description: "Query is utilizing indexes effectively and executed within optimal time bounds.",
			Impact:      "Low latency, efficient resource utilization.",
			Remedy:      "Maintain index maintenance and monitor under production load.",
		})
	}
}
