package config

import (
	"context"
	"os"

	"github.com/sql-doctor/sql-doctor/internal/storage"
)

// Config holds runtime configuration settings
type Config struct {
	GeminiKey      string
	GeminiModel    string
	ActiveConnName string
	OutputFormat   string // "text" or "json"
	Verbose        bool
}

// LoadConfig resolves configuration parameters from env vars and local storage
func LoadConfig(ctx context.Context, store *storage.Storage) (*Config, error) {
	cfg := &Config{
		GeminiModel:  "gemini-2.5-flash",
		OutputFormat: "text",
	}

	// 1. Gemini Key
	if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
		cfg.GeminiKey = envKey
	} else if store != nil {
		if storedKey, _ := store.GetSetting(ctx, "gemini_api_key"); storedKey != "" {
			cfg.GeminiKey = storedKey
		}
	}

	// 2. Gemini Model
	if envModel := os.Getenv("GEMINI_MODEL"); envModel != "" {
		cfg.GeminiModel = envModel
	} else if store != nil {
		if storedModel, _ := store.GetSetting(ctx, "gemini_model"); storedModel != "" {
			cfg.GeminiModel = storedModel
		}
	}

	// 3. Active Connection Name
	if envConn := os.Getenv("SQL_DOCTOR_CONN"); envConn != "" {
		cfg.ActiveConnName = envConn
	} else if store != nil {
		if active, _ := store.GetActiveConnection(ctx); active != nil {
			cfg.ActiveConnName = active.Name
		}
	}

	return cfg, nil
}

// SetGeminiKey updates the persisted Gemini API key
func SetGeminiKey(ctx context.Context, store *storage.Storage, key string) error {
	return store.SetSetting(ctx, "gemini_api_key", key)
}

// SetGeminiModel updates the persisted Gemini Model name
func SetGeminiModel(ctx context.Context, store *storage.Storage, model string) error {
	return store.SetSetting(ctx, "gemini_model", model)
}
