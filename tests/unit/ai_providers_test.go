package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sql-doctor/sql-doctor/internal/ai"
	"github.com/sql-doctor/sql-doctor/internal/ai/claude"
	"github.com/sql-doctor/sql-doctor/internal/ai/gemini"
	"github.com/sql-doctor/sql-doctor/internal/ai/openai"
	"github.com/sql-doctor/sql-doctor/internal/cli"
	"github.com/sql-doctor/sql-doctor/internal/config"
	"github.com/sql-doctor/sql-doctor/internal/storage"
)

func TestAIProviderFactory(t *testing.T) {
	// Gemini default
	pGemini := cli.NewAIProvider(ai.ProviderParams{
		Provider: "gemini",
		APIKey:   "test-gemini-key",
	})
	if pGemini.ProviderName() != "Google Gemini" {
		t.Errorf("expected 'Google Gemini', got '%s'", pGemini.ProviderName())
	}
	if pGemini.Model() != "gemini-3.8-flash" {
		t.Errorf("expected default 'gemini-3.8-flash', got '%s'", pGemini.Model())
	}
	if !pGemini.IsConfigured() {
		t.Errorf("expected gemini to be configured with api key")
	}

	// OpenAI
	pOpenAI := cli.NewAIProvider(ai.ProviderParams{
		Provider: "openai",
		APIKey:   "sk-test-key",
		Model:    "gpt-4o",
	})
	if pOpenAI.ProviderName() != "OpenAI" {
		t.Errorf("expected 'OpenAI', got '%s'", pOpenAI.ProviderName())
	}
	if pOpenAI.Model() != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got '%s'", pOpenAI.Model())
	}
	if !pOpenAI.IsConfigured() {
		t.Errorf("expected openai to be configured")
	}

	// Claude
	pClaude := cli.NewAIProvider(ai.ProviderParams{
		Provider: "claude",
		APIKey:   "sk-ant-test",
		Model:    "claude-3-5-sonnet-20241022",
	})
	if pClaude.ProviderName() != "Anthropic Claude" {
		t.Errorf("expected 'Anthropic Claude', got '%s'", pClaude.ProviderName())
	}
	if pClaude.Model() != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected claude model, got '%s'", pClaude.Model())
	}

	// Ollama
	pOllama := cli.NewAIProvider(ai.ProviderParams{
		Provider: "ollama",
		Endpoint: "http://localhost:11434/v1",
	})
	if pOllama.ProviderName() != "Ollama (Local)" {
		t.Errorf("expected 'Ollama (Local)', got '%s'", pOllama.ProviderName())
	}
	if pOllama.Model() != "deepseek-r1:8b" {
		t.Errorf("expected default 'deepseek-r1:8b', got '%s'", pOllama.Model())
	}
	if !pOllama.IsConfigured() {
		t.Errorf("expected ollama with endpoint to be configured")
	}
}

func TestOpenAIClientMock(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("missing or invalid authorization header")
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "PONG",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := openai.New("test-secret", "gpt-4o-mini", server.URL, false)
	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestGeminiClientMock(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "gemini-test-secret" {
			t.Errorf("missing or invalid x-goog-api-key header")
		}

		resp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"role": "model",
						"parts": []map[string]interface{}{
							{
								"text":    "Thinking through...",
								"thought": true,
							},
							{
								"text":    "PONG",
								"thought": false,
							},
						},
					},
					"finishReason": "STOP",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := gemini.NewWithEndpoint("gemini-test-secret", "gemini-3.8-flash", server.URL)
	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestClaudeClientMock(t *testing.T) {
	client := claude.New("ant-test-secret", "claude-3-5-haiku-20241022")
	if !client.IsConfigured() {
		t.Fatalf("expected client to be configured")
	}
	if client.ProviderName() != "Anthropic Claude" {
		t.Errorf("expected 'Anthropic Claude', got '%s'", client.ProviderName())
	}
}

func TestStorageDeleteSetting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_settings.db")

	store, err := storage.OpenStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	// 1. Set key
	if err := config.SetAIKey(ctx, store, "openai", "sk-proj-12345"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	val, err := store.GetSetting(ctx, "openai_api_key")
	if err != nil || val != "sk-proj-12345" {
		t.Errorf("expected 'sk-proj-12345', got '%s'", val)
	}

	// 2. Clear key
	if err := config.ClearAIKey(ctx, store, "openai"); err != nil {
		t.Fatalf("failed to clear key: %v", err)
	}

	valAfter, err := store.GetSetting(ctx, "openai_api_key")
	if err != nil || valAfter != "" {
		t.Errorf("expected empty string after deletion, got '%s'", valAfter)
	}
}

func TestCuratedModelsByProvider(t *testing.T) {
	providers := []string{ai.ProviderGemini, ai.ProviderOpenAI, ai.ProviderClaude, ai.ProviderOllama}
	for _, p := range providers {
		models, ok := ai.CuratedModelsByProvider[p]
		if !ok || len(models) == 0 {
			t.Errorf("expected curated models for provider %s", p)
		}
		hasRecommended := false
		for _, m := range models {
			if m.Recommended {
				hasRecommended = true
				break
			}
		}
		if !hasRecommended {
			t.Errorf("expected at least one recommended model for provider %s", p)
		}
	}
}
