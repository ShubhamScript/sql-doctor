package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sql-doctor/sql-doctor/internal/ai"
	"github.com/sql-doctor/sql-doctor/internal/config"
	"github.com/sql-doctor/sql-doctor/internal/ui"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage SQL Doctor configuration and AI settings",
}

var configAICmd = &cobra.Command{
	Use:   "ai",
	Short: "View and manage multi-model AI configuration",
	Long: `Display the AI configuration dashboard across all supported providers (Gemini, OpenAI, Claude, Ollama),
switch active models, configure credentials, and run connectivity tests.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if appConfig == nil {
			var err error
			appConfig, err = config.LoadConfig(ctx, appStorage)
			if err != nil {
				return err
			}
		}

		provider := GetAIProvider()
		configured := provider.IsConfigured()

		providerList := []map[string]interface{}{
			{
				"id":         ai.ProviderGemini,
				"name":       "Google Gemini",
				"active":     appConfig.AIProvider == ai.ProviderGemini || appConfig.AIProvider == "",
				"model":      appConfig.GeminiModel,
				"configured": appConfig.GeminiKey != "",
				"key":        maskKey(appConfig.GeminiKey),
				"source":     keySource(appConfig.GeminiKey, "GEMINI_API_KEY"),
			},
			{
				"id":         ai.ProviderOpenAI,
				"name":       "OpenAI (ChatGPT)",
				"active":     appConfig.AIProvider == ai.ProviderOpenAI,
				"model":      appConfig.OpenAIModel,
				"configured": appConfig.OpenAIKey != "",
				"key":        maskKey(appConfig.OpenAIKey),
				"source":     keySource(appConfig.OpenAIKey, "OPENAI_API_KEY"),
			},
			{
				"id":         ai.ProviderClaude,
				"name":       "Anthropic Claude",
				"active":     appConfig.AIProvider == ai.ProviderClaude || appConfig.AIProvider == "anthropic",
				"model":      appConfig.ClaudeModel,
				"configured": appConfig.ClaudeKey != "",
				"key":        maskKey(appConfig.ClaudeKey),
				"source":     keySource(appConfig.ClaudeKey, "ANTHROPIC_API_KEY", "CLAUDE_API_KEY"),
			},
			{
				"id":         ai.ProviderOllama,
				"name":       "Ollama (Local / OpenAI-compatible)",
				"active":     appConfig.AIProvider == ai.ProviderOllama || appConfig.AIProvider == "local",
				"model":      appConfig.OllamaModel,
				"configured": appConfig.OllamaEndpoint != "",
				"key":        maskKey(appConfig.OllamaKey),
				"endpoint":   appConfig.OllamaEndpoint,
				"source":     appConfig.OllamaEndpoint,
			},
		}

		OutputResult(map[string]interface{}{
			"active_provider": appConfig.AIProvider,
			"active_model":    provider.Model(),
			"ai_ready":        configured,
			"providers":       providerList,
		}, func() {
			renderAIDashboard(appConfig, provider, providerList)
		})

		// If running in an interactive terminal and not JSON, offer action menu
		if !flagJSON && term.IsTerminal(int(os.Stdin.Fd())) {
			reader := bufio.NewReader(os.Stdin)
			return promptInteractiveAIMenu(ctx, reader)
		}

		return nil
	},
}

func renderAIDashboard(cfg *config.Config, activeProvider ai.AIProvider, list []map[string]interface{}) {
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("SQL Doctor — AI Configuration Dashboard"))

	activeName := "Google Gemini"
	for _, p := range list {
		if p["active"].(bool) {
			activeName = p["name"].(string)
			break
		}
	}

	statusBadge := ui.CriticalBadge
	if activeProvider.IsConfigured() {
		statusBadge = ui.SuccessBadge
	}

	fmt.Printf("Active Engine: %s  %s  (Model: %s)\n\n",
		ui.HeaderStyle.Render(activeName),
		statusBadge,
		ui.WarningBadge+" "+activeProvider.Model(),
	)

	tbl := ui.NewTable("ACTIVE", "PROVIDER", "STATUS", "ACTIVE MODEL", "SOURCE / ENDPOINT", "API KEY")
	for _, p := range list {
		marker := ""
		if p["active"].(bool) {
			marker = "  ★  "
		}
		status := ui.Error("Not Set")
		if p["configured"].(bool) {
			status = ui.Success("Ready")
		}
		endpointOrSrc := p["source"].(string)
		if ep, ok := p["endpoint"].(string); ok && ep != "" {
			endpointOrSrc = ep
		}
		tbl.AddRow(marker, p["name"].(string), status, p["model"].(string), endpointOrSrc, p["key"].(string))
	}
	fmt.Println(tbl.Render())
	fmt.Println()
}

func promptInteractiveAIMenu(ctx context.Context, reader *bufio.Reader) error {
	fmt.Println(ui.HeaderStyle.Render("Select an action:"))
	fmt.Println("  [1] Switch active provider & model")
	fmt.Println("  [2] Configure API key / endpoint")
	fmt.Println("  [3] Test AI connection")
	fmt.Println("  [4] Delete / Clear API key")
	fmt.Println("  [5] Exit")
	fmt.Println()
	fmt.Print("Enter choice [1-5] (default 5): ")

	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(input)
	if choice == "" || choice == "5" || choice == "exit" || choice == "q" {
		return nil
	}

	switch choice {
	case "1":
		return runInteractiveSwitch(ctx, reader)
	case "2":
		return runInteractiveSetKey(ctx, reader)
	case "3":
		return runTestConnection(ctx, appConfig.AIProvider)
	case "4":
		return runInteractiveClearKey(ctx, reader)
	default:
		fmt.Println(ui.Warning("Invalid option."))
		return nil
	}
}

func runInteractiveSwitch(ctx context.Context, reader *bufio.Reader) error {
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("Select AI Provider to Activate:"))
	fmt.Println("  [1] Google Gemini")
	fmt.Println("  [2] OpenAI (ChatGPT)")
	fmt.Println("  [3] Anthropic Claude")
	fmt.Println("  [4] Ollama (Local / OpenAI-compatible)")
	fmt.Println()
	fmt.Print("Enter choice [1-4] (default 1): ")

	input, _ := reader.ReadString('\n')
	c := strings.TrimSpace(input)
	if c == "" {
		c = "1"
	}

	var targetProvider string
	switch c {
	case "1":
		targetProvider = ai.ProviderGemini
	case "2":
		targetProvider = ai.ProviderOpenAI
	case "3":
		targetProvider = ai.ProviderClaude
	case "4":
		targetProvider = ai.ProviderOllama
	default:
		return fmt.Errorf("invalid provider selection")
	}

	curated := ai.CuratedModelsByProvider[targetProvider]
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("Select Model for " + targetProvider + ":"))
	for i, m := range curated {
		rec := ""
		if m.Recommended {
			rec = " (Recommended)"
		}
		fmt.Printf("  [%d] %-28s %s%s\n", i+1, m.ID, m.Description, rec)
	}
	customIdx := len(curated) + 1
	fmt.Printf("  [%d] Custom model name...\n", customIdx)
	fmt.Println()
	fmt.Printf("Enter choice [1-%d] (default 1): ", customIdx)

	mInput, _ := reader.ReadString('\n')
	mChoice := strings.TrimSpace(mInput)
	if mChoice == "" {
		mChoice = "1"
	}

	var targetModel string
	idx, err := strconv.Atoi(mChoice)
	if err == nil && idx >= 1 && idx <= len(curated) {
		targetModel = curated[idx-1].ID
	} else if idx == customIdx {
		fmt.Print("Enter custom model name: ")
		custInput, _ := reader.ReadString('\n')
		targetModel = strings.TrimSpace(custInput)
	} else {
		targetModel = curated[0].ID
	}

	if err := config.SetAIProvider(ctx, appStorage, targetProvider); err != nil {
		return err
	}
	if err := config.SetAIModel(ctx, appStorage, targetProvider, targetModel); err != nil {
		return err
	}

	// Reload config
	appConfig, _ = config.LoadConfig(ctx, appStorage)
	fmt.Println()
	fmt.Println(ui.Success("Active AI provider switched to '%s' with model '%s'", targetProvider, targetModel))
	return nil
}

func runInteractiveSetKey(ctx context.Context, reader *bufio.Reader) error {
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("Configure Credentials for Provider:"))
	fmt.Println("  [1] Google Gemini")
	fmt.Println("  [2] OpenAI")
	fmt.Println("  [3] Anthropic Claude")
	fmt.Println("  [4] Ollama (Local / Custom Endpoint)")
	fmt.Println()
	fmt.Print("Enter choice [1-4] (default 1): ")

	input, _ := reader.ReadString('\n')
	c := strings.TrimSpace(input)
	if c == "" {
		c = "1"
	}

	var targetProvider string
	switch c {
	case "1":
		targetProvider = ai.ProviderGemini
	case "2":
		targetProvider = ai.ProviderOpenAI
	case "3":
		targetProvider = ai.ProviderClaude
	case "4":
		targetProvider = ai.ProviderOllama
	default:
		return fmt.Errorf("invalid provider selection")
	}

	if targetProvider == ai.ProviderOllama {
		defaultEndpoint := "http://localhost:11434/v1"
		if appConfig.OllamaEndpoint != "" {
			defaultEndpoint = appConfig.OllamaEndpoint
		}
		fmt.Printf("Ollama API Endpoint [%s]: ", defaultEndpoint)
		epInput, _ := reader.ReadString('\n')
		endpoint := strings.TrimSpace(epInput)
		if endpoint == "" {
			endpoint = defaultEndpoint
		}
		if err := config.SetAIEndpoint(ctx, appStorage, endpoint); err != nil {
			return err
		}
		fmt.Println(ui.Success("Ollama endpoint set to '%s'", endpoint))
		return nil
	}

	fmt.Printf("Enter %s API Key (input will be hidden): ", targetProvider)
	byteKey, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read API key: %w", err)
	}

	key := strings.TrimSpace(string(byteKey))
	if key == "" {
		fmt.Println(ui.Warning("API key was empty. Nothing saved."))
		return nil
	}

	if err := config.SetAIKey(ctx, appStorage, targetProvider, key); err != nil {
		return err
	}

	appConfig, _ = config.LoadConfig(ctx, appStorage)
	fmt.Println(ui.Success("%s API key saved successfully (%s)", targetProvider, maskKey(key)))
	return nil
}

func runInteractiveClearKey(ctx context.Context, reader *bufio.Reader) error {
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("Select API Key to Clear / Delete:"))
	fmt.Println("  [1] Google Gemini")
	fmt.Println("  [2] OpenAI")
	fmt.Println("  [3] Anthropic Claude")
	fmt.Println("  [4] Ollama (Endpoint & Key)")
	fmt.Println()
	fmt.Print("Enter choice [1-4]: ")

	input, _ := reader.ReadString('\n')
	c := strings.TrimSpace(input)
	var targetProvider string
	switch c {
	case "1":
		targetProvider = ai.ProviderGemini
	case "2":
		targetProvider = ai.ProviderOpenAI
	case "3":
		targetProvider = ai.ProviderClaude
	case "4":
		targetProvider = ai.ProviderOllama
	default:
		return fmt.Errorf("invalid choice")
	}

	if err := config.ClearAIKey(ctx, appStorage, targetProvider); err != nil {
		return err
	}

	appConfig, _ = config.LoadConfig(ctx, appStorage)
	fmt.Println(ui.Success("Cleared stored credentials for %s.", targetProvider))
	return nil
}

func runTestConnection(ctx context.Context, providerName string) error {
	if providerName == "" {
		providerName = appConfig.AIProvider
	}
	if providerName == "" {
		providerName = ai.ProviderGemini
	}

	fmt.Printf("Testing connection to %s...\n", ui.HeaderStyle.Render(providerName))
	p := GetAIProvider()
	if !p.IsConfigured() {
		return fmt.Errorf("provider %s is not configured with an API key or endpoint", providerName)
	}

	start := time.Now()
	err := p.TestConnection(ctx)
	duration := time.Since(start)

	if err != nil {
		fmt.Println(ui.Error("Connection test failed: %v", err))
		return err
	}

	fmt.Println(ui.Success("Connected to %s (%s) successfully! Latency: %.2f ms", p.ProviderName(), p.Model(), float64(duration.Microseconds())/1000.0))
	return nil
}

// Subcommands for direct CLI usage

var configAISwitchCmd = &cobra.Command{
	Use:   "switch [provider] [model]",
	Short: "Switch active AI provider and model (gemini, openai, claude, ollama)",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if len(args) == 0 {
			reader := bufio.NewReader(os.Stdin)
			return runInteractiveSwitch(ctx, reader)
		}

		targetProvider := args[0]
		targetModel := ""
		if len(args) > 1 {
			targetModel = args[1]
		}

		if err := config.SetAIProvider(ctx, appStorage, targetProvider); err != nil {
			return err
		}
		if targetModel != "" {
			if err := config.SetAIModel(ctx, appStorage, targetProvider, targetModel); err != nil {
				return err
			}
		}

		appConfig, _ = config.LoadConfig(ctx, appStorage)
		OutputResult(map[string]string{
			"provider": targetProvider,
			"model":    appConfig.ActiveAIParams().Model,
		}, func() {
			fmt.Println(ui.Success("Switched AI provider to '%s' (model: %s)", targetProvider, appConfig.ActiveAIParams().Model))
		})
		return nil
	},
}

var configAISetKeyCmd = &cobra.Command{
	Use:   "set-key [provider] [key]",
	Short: "Configure API key for an AI provider (gemini, openai, claude)",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		targetProvider := appConfig.AIProvider
		var key string

		if len(args) == 1 {
			// If 1 arg, check if it's a provider name or key
			lower := strings.ToLower(args[0])
			if lower == "gemini" || lower == "openai" || lower == "claude" || lower == "ollama" {
				targetProvider = lower
			} else {
				key = args[0]
			}
		} else if len(args) >= 2 {
			targetProvider = args[0]
			key = args[1]
		}

		if key == "" {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("API key required. Usage: sql-doctor config ai set-key <provider> <key>")
			}
			fmt.Printf("Enter %s API Key (input will be hidden): ", targetProvider)
			byteKey, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return err
			}
			key = strings.TrimSpace(string(byteKey))
			if key == "" {
				return fmt.Errorf("API key cannot be empty")
			}
		}

		if err := config.SetAIKey(ctx, appStorage, targetProvider, key); err != nil {
			return err
		}

		appConfig, _ = config.LoadConfig(ctx, appStorage)
		OutputResult(map[string]string{
			"provider": targetProvider,
			"key":      maskKey(key),
		}, func() {
			fmt.Println(ui.Success("%s API key saved successfully (%s)", targetProvider, maskKey(key)))
		})
		return nil
	},
}

var configAISetModelCmd = &cobra.Command{
	Use:   "set-model [provider] <model-name>",
	Short: "Set the model name for a provider",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		targetProvider := appConfig.AIProvider
		targetModel := ""

		if len(args) == 1 {
			targetModel = args[0]
		} else {
			targetProvider = args[0]
			targetModel = args[1]
		}

		if err := config.SetAIModel(ctx, appStorage, targetProvider, targetModel); err != nil {
			return err
		}

		appConfig, _ = config.LoadConfig(ctx, appStorage)
		OutputResult(map[string]string{
			"provider": targetProvider,
			"model":    targetModel,
		}, func() {
			fmt.Println(ui.Success("%s model set to '%s'", targetProvider, targetModel))
		})
		return nil
	},
}

var configAISetEndpointCmd = &cobra.Command{
	Use:   "set-endpoint <url>",
	Short: "Set custom endpoint URL for Ollama / local LLM (e.g. http://localhost:11434/v1)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		endpoint := strings.TrimSpace(args[0])
		if err := config.SetAIEndpoint(ctx, appStorage, endpoint); err != nil {
			return err
		}

		appConfig, _ = config.LoadConfig(ctx, appStorage)
		OutputResult(map[string]string{
			"endpoint": endpoint,
		}, func() {
			fmt.Println(ui.Success("Ollama / local LLM endpoint set to '%s'", endpoint))
		})
		return nil
	},
}

var configAITestCmd = &cobra.Command{
	Use:   "test [provider]",
	Short: "Test connection and latency to active or specified AI provider",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := appConfig.AIProvider
		if len(args) > 0 {
			target = args[0]
		}
		return runTestConnection(cmd.Context(), target)
	},
}

var configAIClearCmd = &cobra.Command{
	Use:     "clear [provider]",
	Aliases: []string{"delete", "reset"},
	Short:   "Clear saved API credentials for an AI provider",
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		target := appConfig.AIProvider
		if len(args) > 0 {
			target = args[0]
		}

		if err := config.ClearAIKey(ctx, appStorage, target); err != nil {
			return err
		}

		appConfig, _ = config.LoadConfig(ctx, appStorage)
		OutputResult(map[string]string{
			"cleared": target,
		}, func() {
			fmt.Println(ui.Success("Cleared stored credentials for %s.", target))
		})
		return nil
	},
}

// Backward compatibility commands

var configSetAIKeyCmd = &cobra.Command{
	Use:   "set-ai-key <api-key>",
	Short: "Configure your Gemini / active AI provider API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return configAISetKeyCmd.RunE(cmd, []string{appConfig.AIProvider, args[0]})
	},
}

var configSetModelCmd = &cobra.Command{
	Use:   "set-model <model-name>",
	Short: "Set the model name for the active AI provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return configAISetModelCmd.RunE(cmd, []string{appConfig.AIProvider, args[0]})
	},
}

func maskKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return "(not set)"
	}
	if len(k) <= 8 {
		return "********"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func keySource(storedKey string, envVars ...string) string {
	for _, env := range envVars {
		if os.Getenv(env) != "" {
			return "Environment (" + env + ")"
		}
	}
	if storedKey != "" {
		return "Saved (~/.sql-doctor)"
	}
	return "None"
}

func init() {
	// Register subcommands under `config ai`
	configAICmd.AddCommand(configAISwitchCmd)
	configAICmd.AddCommand(configAISetKeyCmd)
	configAICmd.AddCommand(configAISetModelCmd)
	configAICmd.AddCommand(configAISetEndpointCmd)
	configAICmd.AddCommand(configAITestCmd)
	configAICmd.AddCommand(configAIClearCmd)

	// Register under `config`
	configCmd.AddCommand(configAICmd)
	configCmd.AddCommand(configSetAIKeyCmd)
	configCmd.AddCommand(configSetModelCmd)
}
