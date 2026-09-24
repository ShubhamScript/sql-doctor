package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/data"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Data quality analysis and data cleanliness diagnostics",
}

var dataQualityCmd = &cobra.Command{
	Use:   "quality <table>",
	Short: "Analyze data quality, null percentage, duplicate keys, and formatting anomalies",
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

		analyzer := data.NewQualityAnalyzer(driver)
		report, err := analyzer.AnalyzeTable(ctx, db, tableName)
		if err != nil {
			return err
		}

		OutputResult(report, func() {
			fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("Data Quality Analysis: [%s]", tableName)))
			fmt.Printf("Sampled %d rows\n\n", report.TotalSampled)

			fmt.Println("Data Quality Score:")
			fmt.Println(ui.RenderScoreMeter(report.QualityScore))
			fmt.Println()

			if len(report.Anomalies) > 0 {
				fmt.Println(ui.HeaderStyle.Render("Detected Data Anomalies:"))
				for _, anom := range report.Anomalies {
					badge := ui.InfoBadge
					if anom.Level == "CRITICAL" {
						badge = ui.CriticalBadge
					} else if anom.Level == "WARNING" {
						badge = ui.WarningBadge
					}
					fmt.Printf("\n%s Column '%s' — %s\n", badge, anom.Column, anom.AnomalyType)
					fmt.Printf("  %s\n", anom.Description)
				}
			} else {
				fmt.Println(ui.Success("No data cleanliness anomalies detected in sampled rows."))
			}
		})

		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataQualityCmd)
}
