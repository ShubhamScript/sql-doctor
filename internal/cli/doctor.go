package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/data"
	"github.com/sql-doctor/sql-doctor/internal/schema"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

type DoctorReport struct {
	DatabaseName string               `json:"database_name"`
	Dialect      string               `json:"dialect"`
	ServerVer    string               `json:"server_version"`
	HealthScore  int                  `json:"health_score"`
	Schema       *schema.SchemaReport `json:"schema_report"`
	QualityScore int                  `json:"quality_score"`
	Criticals    []string             `json:"critical_issues"`
	Warnings     []string             `json:"warnings"`
	Summary      string               `json:"summary"`
	AISummary    string               `json:"ai_summary,omitempty"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run comprehensive diagnostic health check across the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		ver, _ := driver.Version(ctx, db)

		report := &DoctorReport{
			DatabaseName: cfg.Database,
			Dialect:      string(driver.Dialect()),
			ServerVer:    ver,
			HealthScore:  100,
		}

		// 1. Schema Analysis
		schemaAnalyzer := schema.NewSchemaAnalyzer(driver)
		sReport, err := schemaAnalyzer.Analyze(ctx, db)
		if err == nil && sReport != nil {
			report.Schema = sReport
			for _, iss := range sReport.Issues {
				msg := fmt.Sprintf("[%s] %s: %s", iss.TableName, iss.Issue, iss.Description)
				if iss.Level == "CRITICAL" {
					report.Criticals = append(report.Criticals, msg)
				} else {
					report.Warnings = append(report.Warnings, msg)
				}
			}
		}

		// 2. Data Quality Sampling on Tables
		tables, _ := driver.Tables(ctx, db)
		qualityAnalyzer := data.NewQualityAnalyzer(driver)
		qualityTotal := 0
		qualityCount := 0

		for _, t := range tables {
			if t.Type == "VIEW" {
				continue
			}
			qReport, err := qualityAnalyzer.AnalyzeTable(ctx, db, t.Name)
			if err == nil && qReport != nil {
				qualityTotal += qReport.QualityScore
				qualityCount++
				for _, anom := range qReport.Anomalies {
					msg := fmt.Sprintf("[%s.%s] %s: %s", t.Name, anom.Column, anom.AnomalyType, anom.Description)
					if anom.Level == "CRITICAL" {
						report.Criticals = append(report.Criticals, msg)
					} else {
						report.Warnings = append(report.Warnings, msg)
					}
				}
			}
		}

		if qualityCount > 0 {
			report.QualityScore = qualityTotal / qualityCount
		} else {
			report.QualityScore = 100
		}

		// 3. Composite Health Score calculation
		schemaWeight := 0.6
		qualityWeight := 0.4
		schemaScore := 100
		if sReport != nil {
			schemaScore = sReport.HealthScore
		}
		compositeScore := int(float64(schemaScore)*schemaWeight + float64(report.QualityScore)*qualityWeight)
		if compositeScore < 0 {
			compositeScore = 0
		}
		report.HealthScore = compositeScore

		// 4. Optional AI Executive Summary
		if flagAI {
			provider := GetAIProvider()
			if provider.IsConfigured() {
				fmt.Println(ui.Info("Generating AI health summary via Gemini..."))
				brief := fmt.Sprintf("Database: %s (%s)\nHealth Score: %d/100\nCriticals (%d):\n%v\nWarnings (%d):\n%v\n",
					cfg.Database, ver, report.HealthScore, len(report.Criticals), report.Criticals, len(report.Warnings), report.Warnings)
				aiSum, err := provider.SummarizeDoctor(ctx, brief)
				if err == nil {
					report.AISummary = aiSum
				}
			} else {
				fmt.Println(ui.Warning("AI summary requested, but Gemini API key is not configured."))
			}
		}

		dbLabel := cfg.Database
		if dbLabel == "" {
			dbLabel = cfg.Name
		}

		OutputResult(report, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("SQL DOCTOR — Full Health Diagnostic Report for [%s]", dbLabel)))
			fmt.Printf("Engine: %s | Dialect: %s\n\n", ver, driver.Dialect())

			fmt.Println("Overall Database Health Score:")
			fmt.Println(ui.RenderScoreMeter(report.HealthScore))
			fmt.Printf("  • Schema Quality:       %d / 100\n", schemaScore)
			fmt.Printf("  • Data Cleanliness:     %d / 100\n\n", report.QualityScore)

			if len(report.Criticals) > 0 {
				fmt.Println(ui.HeaderStyle.Render(fmt.Sprintf("Critical Issues Detected (%d):", len(report.Criticals))))
				for _, c := range report.Criticals {
					fmt.Printf("  %s %s\n", ui.CriticalBadge, c)
				}
				fmt.Println()
			}

			if len(report.Warnings) > 0 {
				fmt.Println(ui.HeaderStyle.Render(fmt.Sprintf("Warnings & Optimization Opportunities (%d):", len(report.Warnings))))
				for _, w := range report.Warnings {
					fmt.Printf("  %s %s\n", ui.WarningBadge, w)
				}
				fmt.Println()
			}

			if len(report.Criticals) == 0 && len(report.Warnings) == 0 {
				fmt.Println(ui.Success("Database passed all diagnostic checks with zero critical issues or warnings!"))
			}

			if report.AISummary != "" {
				fmt.Println(ui.HeaderStyle.Render("Gemini Executive Briefing:"))
				fmt.Println(report.AISummary)
			}
		})

		return nil
	},
}
