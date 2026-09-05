package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/query/parser"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var lintCmd = &cobra.Command{
	Use:   "lint <file.sql | \"<sql>\">",
	Short: "Lint SQL queries for anti-patterns and performance smells",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		input := args[0]
		sqlText := input

		// Check if argument is an existing file
		if data, err := os.ReadFile(input); err == nil {
			sqlText = string(data)
		}

		linter := parser.NewLinter(nil)
		findings, err := linter.Lint(sqlText)
		if err != nil {
			return err
		}

		OutputResult(findings, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Query Linter"))
			if len(findings) == 0 {
				fmt.Println(ui.Success("No SQL lint smells or anti-patterns detected!"))
				return
			}

			fmt.Printf("Found %d lint finding(s):\n", len(findings))
			for _, f := range findings {
				badge := ui.InfoBadge
				if f.Severity == parser.SeverityCritical {
					badge = ui.CriticalBadge
				} else if f.Severity == parser.SeverityWarning {
					badge = ui.WarningBadge
				}
				fmt.Printf("\n%s [%s] %s\n", badge, f.RuleID, f.Title)
				fmt.Printf("  %s\n", f.Description)
				fmt.Printf("  Suggestion: %s\n", f.Suggestion)
			}
		})

		// Return exit code 1 if critical issues exist
		for _, f := range findings {
			if f.Severity == parser.SeverityCritical && !flagJSON {
				os.Exit(1)
			}
		}

		return nil
	},
}
