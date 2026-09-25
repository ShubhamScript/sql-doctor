package config

import (
	"context"
	"os"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/ai"
	"github.com/sql-doctor/sql-doctor/internal/storage"
)

// Config holds runtime configuration settings
type Config struct {
	AIProvider     string // "gemini", "openai", "claude", "ollama"
	GeminiKey      string
	GeminiModel    string
	OpenAIKey      string
	OpenAIModel    string
	ClaudeKey      string
	ClaudeModel    string
	OllamaEndpoint string
	OllamaModel    string
	OllamaKey      string
	ActiveConnName string
	OutputFormat   string // "text" or "json"
	Verbose        bool
}

// ActiveAIParams returns the parameters for initializing the currently active AI provider
func (c *Config) ActiveAIParams() ai.ProviderParams {
	provider := strings.ToLower(strings.TrimSpace(c.AIProvider))
	if provider == "" {
		provider = ai.ProviderGemini
	}

	switch provider {
	case ai.ProviderOpenAI:
		return ai.ProviderParams{
			Provider: ai.ProviderOpenAI,
			Model:    c.OpenAIModel,
			APIKey:   c.OpenAIKey,
		}
	case ai.ProviderClaude, "anthropic":
		return ai.ProviderParams{
			Provider: ai.ProviderClaude,
			Model:    c.ClaudeModel,
			APIKey:   c.ClaudeKey,
		}
	case ai.ProviderOllama, "local":
		return ai.ProviderParams{
			Provider: ai.ProviderOllama,
			Model:    c.OllamaModel,
			APIKey:   c.OllamaKey,
			Endpoint: c.OllamaEndpoint,
		}
	default:
		return ai.ProviderParams{
			Provider: ai.ProviderGemini,
			Model:    c.GeminiModel,
			APIKey:   c.GeminiKey,
		}
	}
}

// LoadConfig resolves configuration parameters from env vars and local storage
func LoadConfig(ctx context.Context, store *storage.Storage) (*Config, error) {
	cfg := &Config{
		AIProvider:     ai.ProviderGemini,
		GeminiModel:    "gemini-3.8-flash",
		OpenAIModel:    "gpt-4o-mini",
		ClaudeModel:    "claude-3-5-haiku-20241022",
		OllamaEndpoint: "http://localhost:11434/v1",
		OllamaModel:    "deepseek-r1:8b",
		OutputFormat:   "text",
	}

	// 1. Active Provider
	if envP := os.Getenv("AI_PROVIDER"); envP != "" {
		cfg.AIProvider = strings.ToLower(envP)
	} else if envP2 := os.Getenv("SQL_DOCTOR_AI_PROVIDER"); envP2 != "" {
		cfg.AIProvider = strings.ToLower(envP2)
	} else if store != nil {
		if storedP, _ := store.GetSetting(ctx, "ai_provider"); storedP != "" {
			cfg.AIProvider = strings.ToLower(storedP)
		}
	}

	// 2. Google Gemini
	if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
		cfg.GeminiKey = envKey
	} else if store != nil {
		if storedKey, _ := store.GetSetting(ctx, "gemini_api_key"); storedKey != "" {
			cfg.GeminiKey = storedKey
		}
	}
	if envModel := os.Getenv("GEMINI_MODEL"); envModel != "" {
		cfg.GeminiModel = envModel
	} else if store != nil {
		if storedModel, _ := store.GetSetting(ctx, "gemini_model"); storedModel != "" {
			cfg.GeminiModel = storedModel
		}
	}
	if cfg.GeminiModel == "gemini-2.5-flash" || cfg.GeminiModel == "gemini-1.5-flash" || cfg.GeminiModel == "gemini-1.5-pro" {
		cfg.GeminiModel = "gemini-3.8-flash"
		if store != nil {
			_ = store.SetSetting(ctx, "gemini_model", "gemini-3.8-flash")
		}
	}

	// 3. OpenAI
	if envKey := os.Getenv("OPENAI_API_KEY"); envKey != "" {
		cfg.OpenAIKey = envKey
	} else if store != nil {
		if storedKey, _ := store.GetSetting(ctx, "openai_api_key"); storedKey != "" {
			cfg.OpenAIKey = storedKey
		}
	}
	if envModel := os.Getenv("OPENAI_MODEL"); envModel != "" {
		cfg.OpenAIModel = envModel
	} else if store != nil {
		if storedModel, _ := store.GetSetting(ctx, "openai_model"); storedModel != "" {
			cfg.OpenAIModel = storedModel
		}
	}

	// 4. Anthropic Claude
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		cfg.ClaudeKey = envKey
	} else if envKey2 := os.Getenv("CLAUDE_API_KEY"); envKey2 != "" {
		cfg.ClaudeKey = envKey2
	} else if store != nil {
		if storedKey, _ := store.GetSetting(ctx, "claude_api_key"); storedKey != "" {
			cfg.ClaudeKey = storedKey
		}
	}
	if envModel := os.Getenv("ANTHROPIC_MODEL"); envModel != "" {
		cfg.ClaudeModel = envModel
	} else if envModel2 := os.Getenv("CLAUDE_MODEL"); envModel2 != "" {
		cfg.ClaudeModel = envModel2
	} else if store != nil {
		if storedModel, _ := store.GetSetting(ctx, "claude_model"); storedModel != "" {
			cfg.ClaudeModel = storedModel
		}
	}

	// 5. Ollama / Local
	if envEndpoint := os.Getenv("OLLAMA_ENDPOINT"); envEndpoint != "" {
		cfg.OllamaEndpoint = envEndpoint
	} else if envEndpoint2 := os.Getenv("OLLAMA_HOST"); envEndpoint2 != "" {
		cfg.OllamaEndpoint = envEndpoint2
	} else if store != nil {
		if storedEndpoint, _ := store.GetSetting(ctx, "ollama_endpoint"); storedEndpoint != "" {
			cfg.OllamaEndpoint = storedEndpoint
		}
	}
	if envModel := os.Getenv("OLLAMA_MODEL"); envModel != "" {
		cfg.OllamaModel = envModel
	} else if store != nil {
		if storedModel, _ := store.GetSetting(ctx, "ollama_model"); storedModel != "" {
			cfg.OllamaModel = storedModel
		}
	}
	if envKey := os.Getenv("OLLAMA_API_KEY"); envKey != "" {
		cfg.OllamaKey = envKey
	} else if store != nil {
		if storedKey, _ := store.GetSetting(ctx, "ollama_api_key"); storedKey != "" {
			cfg.OllamaKey = storedKey
		}
	}

	// 6. Active Connection Name
	if envConn := os.Getenv("SQL_DOCTOR_CONN"); envConn != "" {
		cfg.ActiveConnName = envConn
	} else if store != nil {
		if active, _ := store.GetActiveConnection(ctx); active != nil {
			cfg.ActiveConnName = active.Name
		}
	}

	return cfg, nil
}

// SetAIProvider updates the active AI provider
func SetAIProvider(ctx context.Context, store *storage.Storage, provider string) error {
	return store.SetSetting(ctx, "ai_provider", strings.ToLower(strings.TrimSpace(provider)))
}

// SetAIKey updates the persisted API key for a provider
func SetAIKey(ctx context.Context, store *storage.Storage, provider, key string) error {
	p := normalizeProvider(provider)
	return store.SetSetting(ctx, p+"_api_key", strings.TrimSpace(key))
}

// SetAIModel updates the persisted model for a provider
func SetAIModel(ctx context.Context, store *storage.Storage, provider, model string) error {
	p := normalizeProvider(provider)
	return store.SetSetting(ctx, p+"_model", strings.TrimSpace(model))
}

// SetAIEndpoint updates the persisted endpoint for Ollama / local LLM
func SetAIEndpoint(ctx context.Context, store *storage.Storage, endpoint string) error {
	return store.SetSetting(ctx, "ollama_endpoint", strings.TrimSpace(endpoint))
}

// ClearAIKey deletes the stored API key for a provider
func ClearAIKey(ctx context.Context, store *storage.Storage, provider string) error {
	p := normalizeProvider(provider)
	return store.DeleteSetting(ctx, p+"_api_key")
}

// SetGeminiKey updates the persisted Gemini API key (backward compatibility)
func SetGeminiKey(ctx context.Context, store *storage.Storage, key string) error {
	return SetAIKey(ctx, store, ai.ProviderGemini, key)
}

// SetGeminiModel updates the persisted Gemini Model name (backward compatibility)
func SetGeminiModel(ctx context.Context, store *storage.Storage, model string) error {
	return SetAIModel(ctx, store, ai.ProviderGemini, model)
}

func normalizeProvider(p string) string {
	lower := strings.ToLower(strings.TrimSpace(p))
	switch lower {
	case "google", "gemini":
		return "gemini"
	case "openai", "gpt":
		return "openai"
	case "claude", "anthropic":
		return "claude"
	case "ollama", "local":
		return "ollama"
	default:
		return lower
	}
}
