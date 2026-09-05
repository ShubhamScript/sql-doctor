package ai

import (
	"context"

	"github.com/sql-doctor/sql-doctor/internal/query/analyzer"
)

// GeneratedSQL contains generated SQL and safety precautions
type GeneratedSQL struct {
	SQL          string `json:"sql"`
	Explanation  string `json:"explanation"`
	IsDestructive bool   `json:"is_destructive"`
	Assumptions  string `json:"assumptions"`
}

// AIProvider defines the conversational and AI query generation interface
type AIProvider interface {
	IsConfigured() bool
	Model() string
	ExplainQuery(ctx context.Context, sqlQuery string, metrics *analyzer.QueryAnalysisResult) (string, error)
	OptimizeQuery(ctx context.Context, sqlQuery string, schemaContext string, metrics *analyzer.QueryAnalysisResult) (string, error)
	ReviewSchema(ctx context.Context, schemaSummary string) (string, error)
	Ask(ctx context.Context, question string, schemaContext string) (string, error)
	GenerateSQL(ctx context.Context, prompt string, schemaContext string) (*GeneratedSQL, error)
	SummarizeDoctor(ctx context.Context, doctorSummary string) (string, error)
}
