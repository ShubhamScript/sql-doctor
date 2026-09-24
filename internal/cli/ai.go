package cli

import (
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/ai"
	"github.com/sql-doctor/sql-doctor/internal/ai/claude"
	"github.com/sql-doctor/sql-doctor/internal/ai/gemini"
	"github.com/sql-doctor/sql-doctor/internal/ai/openai"
)

// NewAIProvider creates an AIProvider instance based on provider parameters
func NewAIProvider(p ai.ProviderParams) ai.AIProvider {
	provider := strings.ToLower(strings.TrimSpace(p.Provider))
	if provider == "" {
		provider = ai.ProviderGemini
	}

	switch provider {
	case ai.ProviderOpenAI:
		return openai.New(p.APIKey, p.Model, p.Endpoint, false)

	case ai.ProviderClaude, "anthropic":
		return claude.New(p.APIKey, p.Model)

	case ai.ProviderOllama, "local":
		return openai.New(p.APIKey, p.Model, p.Endpoint, true)

	case ai.ProviderGemini, "google":
		fallthrough
	default:
		return gemini.New(p.APIKey, p.Model)
	}
}
