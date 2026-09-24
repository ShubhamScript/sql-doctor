package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	aiContext "github.com/sql-doctor/sql-doctor/internal/ai/context"
	"github.com/sql-doctor/sql-doctor/internal/query/analyzer"
	"github.com/sql-doctor/sql-doctor/internal/query/explain"
	"github.com/sql-doctor/sql-doctor/internal/query/optimizer"
	"github.com/sql-doctor/sql-doctor/internal/schema"
	"github.com/sql-doctor/sql-doctor/internal/storage"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query performance analysis, execution plan profiling, and index optimization",
}

var queryAnalyzeCmd = &cobra.Command{
	Use:   "analyze \"<sql>\"",
	Short: "Analyze query performance, execution metrics, and full table scans",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		queryText := args[0]

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		anz := analyzer.NewAnalyzer(driver)
		result, err := anz.Analyze(ctx, db, queryText)
		if err != nil {
			return fmt.Errorf("query analysis failed: %w", err)
		}

		// Record in local history
		if appStorage != nil {
			_ = appStorage.RecordQuery(ctx, &storage.QueryHistoryRecord{
				ConnectionName:   cfg.Name,
				QueryText:        queryText,
				Fingerprint:      result.Fingerprint,
				DurationMs:       result.ExecutionTimeMs,
				RowsExamined:     result.RowsExamined,
				RowsReturned:     result.RowsReturned,
				PerformanceScore: result.PerformanceScore,
			})
		}

		var aiExplanation string
		if flagAI {
			provider := GetAIProvider()
			if provider.IsConfigured() {
				fmt.Println(ui.Info("Requesting AI performance explanation from Gemini..."))
				expl, err := provider.ExplainQuery(ctx, queryText, result)
				if err == nil {
					aiExplanation = expl
				}
			} else {
				fmt.Println(ui.Warning("AI explanation requested, but Gemini API key is not configured."))
			}
		}

		OutputResult(map[string]interface{}{
			"analysis":       result,
			"ai_explanation": aiExplanation,
		}, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Query Performance Analysis"))
			fmt.Printf("Query: %s\n\n", queryText)

			fmt.Println("Performance Score:")
			fmt.Println(ui.RenderScoreMeter(result.PerformanceScore))
			fmt.Println()

			tbl := ui.NewTable("METRIC", "VALUE", "EVALUATION")
			tbl.AddRow("Execution Time", fmt.Sprintf("%.2f ms", result.ExecutionTimeMs), timeEval(result.ExecutionTimeMs))
			tbl.AddRow("Rows Returned", fmt.Sprintf("%d", result.RowsReturned), "-")
			tbl.AddRow("Rows Examined", fmt.Sprintf("%d", result.RowsExamined), ratioEval(result.ExaminedRatio))
			tbl.AddRow("Full Table Scan", fmt.Sprintf("%v", result.HasFullTableScan), scanEval(result.HasFullTableScan))
			tbl.AddRow("Filesort Sort", fmt.Sprintf("%v", result.UsesFilesort), boolEval(result.UsesFilesort))
			tbl.AddRow("Temp Table", fmt.Sprintf("%v", result.UsesTempTable), boolEval(result.UsesTempTable))
			fmt.Println(tbl.Render())

			if len(result.Findings) > 0 {
				fmt.Println(ui.HeaderStyle.Render("\nDiagnostic Findings:"))
				for _, f := range result.Findings {
					badge := ui.InfoBadge
					if f.Level == "CRITICAL" {
						badge = ui.CriticalBadge
					} else if f.Level == "WARNING" {
						badge = ui.WarningBadge
					} else if f.Level == "OPTIMAL" {
						badge = ui.SuccessBadge
					}
					fmt.Printf("\n%s %s\n", badge, ui.HeaderStyle.Render(f.Title))
					fmt.Printf("  %s\n", f.Description)
					if f.Remedy != "" {
						fmt.Printf("  Remedy: %s\n", f.Remedy)
					}
				}
			}

			if aiExplanation != "" {
				fmt.Println(ui.HeaderStyle.Render("\nGemini AI Explanation:"))
				fmt.Println(aiExplanation)
			}
		})

		return nil
	},
}

var queryExplainCmd = &cobra.Command{
	Use:   "explain \"<sql>\"",
	Short: "Inspect execution plan tree and identify expensive operations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		queryText := args[0]

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		explainRes, err := driver.Explain(ctx, db, queryText, false)
		if err != nil {
			return fmt.Errorf("failed to explain query: %w", err)
		}

		planAnalyzer := explain.NewPlanAnalyzer()
		summary := planAnalyzer.Analyze(explainRes)

		OutputResult(summary, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Execution Plan Analysis"))
			fmt.Printf("Query: %s\n\n", queryText)

			if summary.FormattedTree != "" {
				fmt.Println(ui.HeaderStyle.Render("Execution Plan Hierarchy:"))
				fmt.Println(summary.FormattedTree)
			}

			fmt.Println(ui.HeaderStyle.Render("Human-Readable Plan Summary:"))
			fmt.Println(summary.HumanSummary)

			if len(summary.ExpensiveNodes) > 0 {
				fmt.Println(ui.HeaderStyle.Render("\nExpensive Plan Operations:"))
				for _, n := range summary.ExpensiveNodes {
					fmt.Printf("  • %s\n", n)
				}
			}
		})
		return nil
	},
}

var queryOptimizeCmd = &cobra.Command{
	Use:   "optimize \"<sql>\"",
	Short: "Recommend optimal composite indexes and query rewrites",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		queryText := args[0]

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		opt := optimizer.NewOptimizer(driver)
		res, err := opt.Optimize(ctx, db, queryText)
		if err != nil {
			return err
		}

		var aiRecommendations string
		if flagAI {
			provider := GetAIProvider()
			if provider.IsConfigured() {
				fmt.Println(ui.Info("Requesting AI optimization recommendations from Gemini..."))
				tables, _ := driver.Tables(ctx, db)
				details, _ := schema.FetchAllTableDetails(ctx, driver, db)
				schemaCtx := aiContext.BuildMinifiedSchema(tables, details)

				anz := analyzer.NewAnalyzer(driver)
				metrics, _ := anz.Analyze(ctx, db, queryText)

				rec, err := provider.OptimizeQuery(ctx, queryText, schemaCtx, metrics)
				if err == nil {
					aiRecommendations = rec
				}
			} else {
				fmt.Println(ui.Warning("AI optimization requested, but Gemini API key is not configured."))
			}
		}

		OutputResult(map[string]interface{}{
			"recommendations":    res,
			"ai_recommendations": aiRecommendations,
		}, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Query Optimizer & Index Advisor"))
			fmt.Printf("Query: %s\n\n", queryText)

			if len(res.IndexRecommendations) > 0 {
				fmt.Println(ui.HeaderStyle.Render("Recommended Indexes:"))
				for _, idx := range res.IndexRecommendations {
					fmt.Printf("\n%s Target: %s (%s)\n", ui.SuccessBadge, idx.TableName, strings.Join(idx.Columns, ", "))
					fmt.Printf("  DDL:      %s\n", lipgloss.NewStyle().Bold(true).Foreground(ui.PrimaryColor).Render(idx.SuggestedDDL))
					fmt.Printf("  Gain:     %s\n", idx.EstimatedGain)
					fmt.Printf("  Rationale: %s\n", idx.Rationale)
					fmt.Printf("  Tradeoff:  %s\n", idx.Tradeoffs)
				}
			} else {
				fmt.Println(ui.Success("No new indexes required for this query."))
			}

			if len(res.RewriteSuggestions) > 0 {
				fmt.Println(ui.HeaderStyle.Render("\nRecommended Query Rewrites:"))
				for _, rw := range res.RewriteSuggestions {
					fmt.Printf("  • Original:  %s\n", rw.OriginalSnippet)
					fmt.Printf("    Suggested: %s\n", rw.SuggestedRewrite)
					fmt.Printf("    Rationale: %s\n", rw.Rationale)
				}
			}

			if aiRecommendations != "" {
				fmt.Println(ui.HeaderStyle.Render("\nGemini AI Recommendations:"))
				fmt.Println(aiRecommendations)
			}
		})

		return nil
	},
}

func timeEval(ms float64) string {
	if ms < 50.0 {
		return "FAST"
	}
	if ms < 250.0 {
		return "ACCEPTABLE"
	}
	return "SLOW"
}

func ratioEval(r float64) string {
	if r == 0 {
		return "OPTIMAL"
	}
	if r > 50.0 {
		return "HIGH (Poor Filtering)"
	}
	return "ACCEPTABLE"
}

func scanEval(fs bool) string {
	if fs {
		return "UNINDEXED TABLE SCAN"
	}
	return "INDEX SCAN"
}

func boolEval(b bool) string {
	if b {
		return "YES (Overhead)"
	}
	return "NO"
}

func init() {
	queryCmd.AddCommand(queryAnalyzeCmd)
	queryCmd.AddCommand(queryExplainCmd)
	queryCmd.AddCommand(queryOptimizeCmd)
}
