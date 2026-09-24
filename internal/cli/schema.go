package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	aiContext "github.com/sql-doctor/sql-doctor/internal/ai/context"
	"github.com/sql-doctor/sql-doctor/internal/schema"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Schema quality analysis and data-aware datatype advisor",
}

var schemaAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze schema design smells, missing PKs, unindexed foreign keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		analyzer := schema.NewSchemaAnalyzer(driver)
		report, err := analyzer.Analyze(ctx, db)
		if err != nil {
			return err
		}

		var aiReview string
		if flagAI {
			provider := GetAIProvider()
			if provider.IsConfigured() {
				fmt.Println(ui.Info("Requesting AI schema review from Gemini..."))
				tables, _ := driver.Tables(ctx, db)
				details, _ := schema.FetchAllTableDetails(ctx, driver, db)
				schemaCtx := aiContext.BuildMinifiedSchema(tables, details)
				rev, err := provider.ReviewSchema(ctx, schemaCtx)
				if err == nil {
					aiReview = rev
				}
			} else {
				fmt.Println(ui.Warning("AI review requested, but Gemini API key is not configured."))
			}
		}

		OutputResult(map[string]interface{}{
			"report":    report,
			"ai_review": aiReview,
		}, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Schema Health Analysis"))
			fmt.Printf("Analyzed %d tables, %d indexes, %d foreign keys\n\n", report.TotalTables, report.TotalIndexes, report.TotalForeignKeys)

			fmt.Println("Schema Health Score:")
			fmt.Println(ui.RenderScoreMeter(report.HealthScore))
			fmt.Println()

			if len(report.Issues) > 0 {
				fmt.Println(ui.HeaderStyle.Render("Identified Schema Issues:"))
				for _, iss := range report.Issues {
					badge := ui.InfoBadge
					if iss.Level == "CRITICAL" {
						badge = ui.CriticalBadge
					} else if iss.Level == "WARNING" {
						badge = ui.WarningBadge
					}
					fmt.Printf("\n%s Table: %s — %s\n", badge, iss.TableName, iss.Issue)
					fmt.Printf("  %s\n", iss.Description)
					fmt.Printf("  Remedy: %s\n", iss.Remedy)
				}
			} else {
				fmt.Println(ui.Success("No major schema design smells detected."))
			}

			if aiReview != "" {
				fmt.Println(ui.HeaderStyle.Render("\nGemini AI Schema Review:"))
				fmt.Println(aiReview)
			}
		})

		return nil
	},
}

var schemaDatatypesCmd = &cobra.Command{
	Use:   "datatypes <table>",
	Short: "Data-aware datatype advisor analyzing observed row contents",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		tableName := args[0]

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		advisor := schema.NewAdvisor(driver)
		recs, err := advisor.AnalyzeTable(ctx, db, tableName)
		if err != nil {
			return err
		}

		OutputResult(recs, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Data-Aware Datatype Advisor: [%s]", tableName)))
			if len(recs) == 0 {
				fmt.Println(ui.Success("All column types align cleanly with observed data characteristics."))
				return
			}

			for _, r := range recs {
				fmt.Printf("\n%s Column '%s'\n", ui.InfoBadge, r.ColumnName)
				fmt.Printf("  Current Type:   %s\n", r.CurrentType)
				fmt.Printf("  Suggested Type: %s (Confidence: %d%%)\n", ui.Success("%s", r.SuggestedType), r.Confidence)
				fmt.Printf("  Observed Stats: %s\n", r.ObservedStats)
				fmt.Printf("  Rationale:      %s\n", r.Rationale)
				if r.FutureWarning != "" {
					fmt.Printf("  Notice:         %s\n", ui.Warning("%s", r.FutureWarning))
				}
			}
		})

		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaAnalyzeCmd)
	schemaCmd.AddCommand(schemaDatatypesCmd)
}
