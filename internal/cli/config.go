package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/config"
	"github.com/sql-doctor/sql-doctor/internal/ui"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage SQL Doctor configuration and AI settings",
}

var configAICmd = &cobra.Command{
	Use:   "ai",
	Short: "Show AI configuration status",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := GetAIProvider()
		configured := provider.IsConfigured()

		OutputResult(map[string]interface{}{
			"ai_enabled":   configured,
			"model":        provider.Model(),
			"provider":     "Google Gemini",
			"key_source":   resolveKeySource(),
		}, func() {
			fmt.Println(ui.TitleStyle.Render("SQL Doctor — AI Configuration Status"))
			if configured {
				fmt.Println(ui.Success("Gemini AI is configured and ready."))
				fmt.Printf("  Provider: Google Gemini\n")
				fmt.Printf("  Model:    %s\n", provider.Model())
				fmt.Printf("  Source:   %s\n", resolveKeySource())
			} else {
				fmt.Println(ui.Warning("Gemini AI is NOT configured."))
				fmt.Println("\nTo enable AI query explanations, natural language questions, and optimizations:")
				fmt.Println("  sql-doctor config set-ai-key <your-gemini-api-key>")
				fmt.Println("or set the environment variable:")
				fmt.Println("  export GEMINI_API_KEY=\"...\"")
			}
		})
		return nil
	},
}

var configSetAIKeyCmd = &cobra.Command{
	Use:   "set-ai-key <api-key>",
	Short: "Configure your Gemini API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}

		key := strings.TrimSpace(args[0])
		if key == "" {
			return fmt.Errorf("API key cannot be empty")
		}

		if err := config.SetGeminiKey(ctx, appStorage, key); err != nil {
			return fmt.Errorf("failed to save API key: %w", err)
		}

		masked := key
		if len(key) > 8 {
			masked = key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
		}

		OutputResult(map[string]interface{}{
			"status": "SAVED",
			"key":    masked,
		}, func() {
			fmt.Println(ui.Success("Gemini API key saved successfully (%s)", masked))
		})
		return nil
	},
}

var configSetModelCmd = &cobra.Command{
	Use:   "set-model <model-name>",
	Short: "Set the Gemini model name (default: gemini-2.5-flash)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if appStorage == nil {
			return fmt.Errorf("local storage unavailable")
		}

		model := strings.TrimSpace(args[0])
		if err := config.SetGeminiModel(ctx, appStorage, model); err != nil {
			return fmt.Errorf("failed to save model: %w", err)
		}

		OutputResult(map[string]interface{}{
			"status": "SAVED",
			"model":  model,
		}, func() {
			fmt.Println(ui.Success("Gemini model set to '%s'", model))
		})
		return nil
	},
}

func resolveKeySource() string {
	if appConfig == nil {
		return "None"
	}
	if appConfig.GeminiKey != "" {
		return "Configured (Stored / Environment Variable)"
	}
	return "None"
}

func init() {
	configCmd.AddCommand(configAICmd)
	configCmd.AddCommand(configSetAIKeyCmd)
	configCmd.AddCommand(configSetModelCmd)
}
