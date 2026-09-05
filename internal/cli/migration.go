package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/migration"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var migrationCmd = &cobra.Command{
	Use:   "migration",
	Short: "Migration safety checks and lock risk diagnostics",
}

var migrationCheckCmd = &cobra.Command{
	Use:   "check <file.sql>",
	Short: "Check migration script for destructive actions, table rewrites, and locks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		analyzer := migration.NewAnalyzer()
		report, err := analyzer.AnalyzeFile(filePath)
		if err != nil {
			return err
		}

		OutputResult(report, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Migration Safety Analysis"))
			fmt.Printf("File: %s\n\n", filePath)

			if report.IsSafe {
				fmt.Println(ui.Success("Migration is deemed SAFE for automated execution."))
			} else {
				fmt.Println(ui.Error("Migration contains CRITICAL safety risks! Manual DBA review required."))
			}

			if len(report.Risks) > 0 {
				fmt.Println(ui.HeaderStyle.Render("\nDetected Hazards:"))
				for _, r := range report.Risks {
					badge := ui.InfoBadge
					if r.RiskLevel == migration.RiskCritical {
						badge = ui.CriticalBadge
					} else if r.RiskLevel == migration.RiskWarning {
						badge = ui.WarningBadge
					}
					fmt.Printf("\n%s Line %d: %s\n", badge, r.LineNumber, r.Hazard)
					fmt.Printf("  Statement: %s\n", r.Statement)
					fmt.Printf("  Risk:      %s\n", r.Description)
					fmt.Printf("  Remedy:    %s\n", r.Remedy)
				}
			}
		})

		return nil
	},
}

func init() {
	migrationCmd.AddCommand(migrationCheckCmd)
}
