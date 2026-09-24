package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/sql-doctor/sql-doctor/internal/query/analyzer"
)

// BaseCaller is a function that makes the actual API call to the LLM backend
type BaseCaller func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// BaseProvider implements the standard AIProvider methods on top of a BaseCaller
type BaseProvider struct {
	Name       string
	ModelName  string
	Configured bool
	Caller     BaseCaller
}

func (b *BaseProvider) IsConfigured() bool {
	return b.Configured
}

func (b *BaseProvider) ProviderName() string {
	return b.Name
}

func (b *BaseProvider) Model() string {
	return b.ModelName
}

func (b *BaseProvider) TestConnection(ctx context.Context) error {
	if !b.Configured {
		return fmt.Errorf("%s is not configured (missing API key or endpoint)", b.Name)
	}
	res, err := b.Caller(ctx, "You are a test ping agent.", "Reply with the single word 'PONG' only.")
	if err != nil {
		return err
	}
	if strings.TrimSpace(res) == "" {
		return fmt.Errorf("received empty response from %s", b.Name)
	}
	return nil
}

func (b *BaseProvider) ExplainQuery(ctx context.Context, sqlQuery string, metrics *analyzer.QueryAnalysisResult) (string, error) {
	sys := "You are a Senior Principal Database Performance Engineer. Provide clear, concise, actionable query analysis."
	var metricsStr string
	if metrics != nil {
		metricsStr = fmt.Sprintf("Execution Time: %.2fms\nRows Examined: %d\nRows Returned: %d\nFull Table Scan: %v\nPerformance Score: %d/100",
			metrics.ExecutionTimeMs, metrics.RowsExamined, metrics.RowsReturned, metrics.HasFullTableScan, metrics.PerformanceScore)
	}

	prompt := fmt.Sprintf(`Explain the execution behavior and performance characteristics of this SQL query:

SQL:
%s

Observed Metrics:
%s

Explain in 2-3 concise paragraphs:
1. What the query is doing logically.
2. Why the query is fast or slow based on the observed metrics.
3. Specific actionable steps to improve it.`, sqlQuery, metricsStr)

	return b.Caller(ctx, sys, prompt)
}

func (b *BaseProvider) OptimizeQuery(ctx context.Context, sqlQuery string, schemaContext string, metrics *analyzer.QueryAnalysisResult) (string, error) {
	sys := "You are an expert SQL Query Optimizer. Always prioritize index selection, sargability, and deterministic query rewrites."
	prompt := fmt.Sprintf(`Analyze and provide concrete optimization recommendations for this SQL query:

QUERY:
%s

SCHEMA CONTEXT:
%s

Provide:
1. Suggested optimized SQL rewrite.
2. Any recommended composite or single-column indexes with exact DDL.
3. Rationale explaining why the rewrite is faster.`, sqlQuery, schemaContext)

	return b.Caller(ctx, sys, prompt)
}

func (b *BaseProvider) ReviewSchema(ctx context.Context, schemaSummary string) (string, error) {
	sys := "You are a Senior Database Architect. Review database schema design, normalization, relationships, and index strategy."
	prompt := fmt.Sprintf(`Review this database schema and identify design smells, missing constraints, or performance hazards:

%s

Provide:
1. Architectural strengths and design quality evaluation.
2. High-priority schema risks or normalization smells.
3. Recommended improvements.`, schemaSummary)

	return b.Caller(ctx, sys, prompt)
}

func (b *BaseProvider) Ask(ctx context.Context, question string, schemaContext string) (string, error) {
	sys := "You are SQL Doctor, an intelligent database diagnostics and engineering assistant. Ground your answer strictly in the provided database schema."
	prompt := fmt.Sprintf(`Question: %s

Connected Database Schema:
%s

Answer the user's question clearly and provide relevant SQL snippets or explanations based strictly on the provided schema.`, question, schemaContext)

	return b.Caller(ctx, sys, prompt)
}

func (b *BaseProvider) GenerateSQL(ctx context.Context, userGoal string, schemaContext string) (*GeneratedSQL, error) {
	sys := "You are an expert SQL developer. Generate valid, high-performance SQL based strictly on the provided schema. Output only the SQL query and brief rationale."
	prompt := fmt.Sprintf(`User Goal: %s

Schema:
%s

Generate the optimal SQL query to accomplish the user's goal.
Format your output as:
---SQL---
<The SQL Query Here>
---EXPLANATION---
<Brief explanation of the logic and any assumptions>
`, userGoal, schemaContext)

	raw, err := b.Caller(ctx, sys, prompt)
	if err != nil {
		return nil, err
	}

	result := &GeneratedSQL{}
	if strings.Contains(raw, "---SQL---") {
		parts := strings.Split(raw, "---SQL---")
		if len(parts) > 1 {
			subParts := strings.Split(parts[1], "---EXPLANATION---")
			result.SQL = CleanCodeBlock(subParts[0])
			if len(subParts) > 1 {
				result.Explanation = strings.TrimSpace(subParts[1])
			}
		}
	} else {
		result.SQL = CleanCodeBlock(raw)
	}

	upper := strings.ToUpper(result.SQL)
	if strings.Contains(upper, "DELETE") || strings.Contains(upper, "UPDATE") || strings.Contains(upper, "DROP") || strings.Contains(upper, "TRUNCATE") {
		result.IsDestructive = true
	}

	return result, nil
}

func (b *BaseProvider) SummarizeDoctor(ctx context.Context, doctorSummary string) (string, error) {
	sys := "You are a Database Reliability Engineer. Provide an executive summary of database health findings."
	prompt := fmt.Sprintf(`Summarize these database diagnostic findings into an executive briefing with prioritized action items:

%s`, doctorSummary)

	return b.Caller(ctx, sys, prompt)
}

// CleanCodeBlock strips markdown backticks from generated code
func CleanCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```sql") {
		s = strings.TrimPrefix(s, "```sql")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
