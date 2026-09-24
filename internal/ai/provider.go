package ai

import (
	"context"

	"github.com/sql-doctor/sql-doctor/internal/query/analyzer"
)

// Supported Provider Constants
const (
	ProviderGemini = "gemini"
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
	ProviderOllama = "ollama"
)

// CuratedModel describes a recommended model option
type CuratedModel struct {
	ID          string
	Name        string
	Description string
	Recommended bool
}

// CuratedModelsByProvider lists recommended models that are well-balanced for SQL tasks
var CuratedModelsByProvider = map[string][]CuratedModel{
	ProviderGemini: {
		{ID: "gemini-3.8-flash", Name: "Gemini 3.8 Flash", Description: "Fast, state-of-the-art reasoning for SQL, code & schema diagnosis", Recommended: true},
		{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", Description: "Balanced high-performance model for diagnostics & query generation"},
		{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash Lite", Description: "Lightweight, ultra-fast latency and cost-efficient"},
	},
	ProviderOpenAI: {
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Description: "Fast, cost-effective, exceptional SQL generation & accuracy", Recommended: true},
		{ID: "gpt-4o", Name: "GPT-4o", Description: "Flagship model with deep multi-step reasoning"},
		{ID: "o3-mini", Name: "o3 Mini", Description: "Advanced reasoning model for complex optimization math"},
	},
	ProviderClaude: {
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Description: "Ultra-fast, cost-effective, precise SQL syntax generation", Recommended: true},
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", Description: "Hybrid reasoning & high-precision architectural analysis"},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", Description: "Industry-leading coding & schema architectural analysis"},
	},
	ProviderOllama: {
		{ID: "deepseek-r1:8b", Name: "DeepSeek R1 (8B)", Description: "Open-weights reasoning model running locally", Recommended: true},
		{ID: "qwen2.5-coder:7b", Name: "Qwen 2.5 Coder (7B)", Description: "Optimized specifically for code and SQL queries"},
		{ID: "llama3.1:8b", Name: "Llama 3.1 (8B)", Description: "Popular general-purpose local LLM"},
	},
}

// GeneratedSQL contains generated SQL and safety precautions
type GeneratedSQL struct {
	SQL           string `json:"sql"`
	Explanation   string `json:"explanation"`
	IsDestructive bool   `json:"is_destructive"`
	Assumptions   string `json:"assumptions"`
}

// ProviderParams defines parameters for initializing an AIProvider
type ProviderParams struct {
	Provider string
	Model    string
	APIKey   string
	Endpoint string
}

// AIProvider defines the conversational and AI query generation interface
type AIProvider interface {
	IsConfigured() bool
	ProviderName() string
	Model() string
	TestConnection(ctx context.Context) error
	ExplainQuery(ctx context.Context, sqlQuery string, metrics *analyzer.QueryAnalysisResult) (string, error)
	OptimizeQuery(ctx context.Context, sqlQuery string, schemaContext string, metrics *analyzer.QueryAnalysisResult) (string, error)
	ReviewSchema(ctx context.Context, schemaSummary string) (string, error)
	Ask(ctx context.Context, question string, schemaContext string) (string, error)
	GenerateSQL(ctx context.Context, prompt string, schemaContext string) (*GeneratedSQL, error)
	SummarizeDoctor(ctx context.Context, doctorSummary string) (string, error)
}
