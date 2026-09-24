package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	aiContext "github.com/sql-doctor/sql-doctor/internal/ai/context"
	"github.com/sql-doctor/sql-doctor/internal/schema"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var (
	askFlagExecute bool
	askFlagYes     bool
)

var askCmd = &cobra.Command{
	Use:   "ask \"<question or natural language request>\"",
	Short: "Ask questions about your database or generate SQL using Gemini AI",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		prompt := args[0]

		provider := GetAIProvider()
		if !provider.IsConfigured() {
			fmt.Println(ui.Warning("AI features are unavailable because a Gemini API key has not been configured."))
			fmt.Println("\nConfigure your own key using:")
			fmt.Println("  sql-doctor config set-ai-key <your-api-key>")
			fmt.Println("or set the environment variable:")
			fmt.Println("  export GEMINI_API_KEY=\"...\"")
			return nil
		}

		db, driver, cfg, err := GetActiveDB(ctx)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := EnsureDatabase(ctx, db, driver, cfg); err != nil {
			return err
		}

		fmt.Println(ui.Info("Inspecting schema context for [%s]...", cfg.Database))
		tables, _ := driver.Tables(ctx, db)
		details, _ := schema.FetchAllTableDetails(ctx, driver, db)
		schemaContext := aiContext.BuildMinifiedSchema(tables, details)

		// Determine if prompt looks like a query generation request
		lowerPrompt := strings.ToLower(prompt)
		isQueryGen := strings.HasPrefix(lowerPrompt, "write") || strings.HasPrefix(lowerPrompt, "generate") ||
			strings.HasPrefix(lowerPrompt, "find") || strings.HasPrefix(lowerPrompt, "select") ||
			strings.HasPrefix(lowerPrompt, "get") || strings.Contains(lowerPrompt, "query to")

		if isQueryGen {
			fmt.Println(ui.Info("Generating SQL with Gemini %s...", provider.Model()))
			generated, err := provider.GenerateSQL(ctx, prompt, schemaContext)
			if err != nil {
				return err
			}

			OutputResult(generated, func() {
				fmt.Println(ui.TitleStyle.Render("SQL Doctor — AI Generated Query"))
				fmt.Printf("Goal: %s\n\n", prompt)

				if generated.IsDestructive {
					fmt.Println(ui.CriticalBadge + " " + ui.Error("This query modifies or deletes data!"))
				}

				fmt.Println(ui.HeaderStyle.Render("Generated SQL:"))
				sqlBox := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(ui.PrimaryColor).
					Padding(1, 2).
					Render(generated.SQL)
				fmt.Println(sqlBox)

				if generated.Explanation != "" {
					fmt.Println(ui.HeaderStyle.Render("\nExplanation:"))
					fmt.Println(generated.Explanation)
				}

				if askFlagExecute {
					if generated.IsDestructive && !askFlagYes {
						fmt.Print(ui.Warning("\nAre you sure you want to execute this destructive query? (y/N): "))
						reader := bufio.NewReader(os.Stdin)
						ans, _ := reader.ReadString('\n')
						if strings.ToLower(strings.TrimSpace(ans)) != "y" {
							fmt.Println(ui.Info("Execution cancelled."))
							return
						}
					}

					fmt.Println(ui.Info("Executing query on [%s]...", cfg.Database))
					rows, err := db.QueryContext(ctx, generated.SQL)
					if err != nil {
						fmt.Println(ui.Error("Query execution failed: %v", err))
						return
					}
					defer rows.Close()
					fmt.Println(ui.Success("Query executed successfully."))
				} else {
					fmt.Println("\nTo run this query directly, use: --execute")
				}
			})
			return nil
		}

		// Conversational Question
		fmt.Println(ui.Info("Consulting Gemini %s...", provider.Model()))
		answer, err := provider.Ask(ctx, prompt, schemaContext)
		if err != nil {
			return err
		}

		OutputResult(map[string]string{
			"question": prompt,
			"answer":   answer,
		}, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — Database Assistant"))
			fmt.Printf("Question: %s\n\n", prompt)
			fmt.Println(answer)
		})

		return nil
	},
}

func init() {
	askCmd.Flags().BoolVar(&askFlagExecute, "execute", false, "Execute the generated SQL against the database")
	askCmd.Flags().BoolVarP(&askFlagYes, "yes", "y", false, "Confirm destructive execution without prompting")
}
