package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/query/parser"
)

var formatCmd = &cobra.Command{
	Use:   "format <file.sql | \"<sql>\">",
	Short: "Format and pretty-print SQL queries",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		input := args[0]
		sqlText := input

		if data, err := os.ReadFile(input); err == nil {
			sqlText = string(data)
		}

		formatter := parser.NewFormatter()
		formatted := formatter.Format(sqlText)

		OutputResult(map[string]string{
			"formatted": formatted,
		}, func() {
			fmt.Println(formatted)
		})

		return nil
	},
}
