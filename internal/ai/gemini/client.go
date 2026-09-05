package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/sql-doctor/sql-doctor/internal/ai"
	"github.com/sql-doctor/sql-doctor/internal/query/analyzer"
	"google.golang.org/api/option"
)

// Client implements ai.AIProvider via the official Google Gemini Go SDK
type Client struct {
	apiKey    string
	modelName string
}

func New(apiKey, modelName string) *Client {
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	return &Client{
		apiKey:    apiKey,
		modelName: modelName,
	}
}

func (c *Client) IsConfigured() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

func (c *Client) Model() string {
	return c.modelName
}

func (c *Client) missingKeyErr() error {
	return fmt.Errorf("AI features are unavailable because a Gemini API key has not been configured.\n\nConfigure your key using:\n  sql-doctor config set-ai-key\nor set the GEMINI_API_KEY environment variable")
}

func (c *Client) callGemini(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !c.IsConfigured() {
		return "", c.missingKeyErr()
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to initialize Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(c.modelName)
	if systemPrompt != "" {
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(systemPrompt)},
		}
	}

	resp, err := model.GenerateContent(ctx, genai.Text(userPrompt))
	if err != nil {
		return "", fmt.Errorf("Gemini API error: %w", err)
	}

	var sb strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					sb.WriteString(string(txt))
				}
			}
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

func (c *Client) ExplainQuery(ctx context.Context, sqlQuery string, metrics *analyzer.QueryAnalysisResult) (string, error) {
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

	return c.callGemini(ctx, sys, prompt)
}

func (c *Client) OptimizeQuery(ctx context.Context, sqlQuery string, schemaContext string, metrics *analyzer.QueryAnalysisResult) (string, error) {
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

	return c.callGemini(ctx, sys, prompt)
}

func (c *Client) ReviewSchema(ctx context.Context, schemaSummary string) (string, error) {
	sys := "You are a Senior Database Architect. Review database schema design, normalization, relationships, and index strategy."
	prompt := fmt.Sprintf(`Review this database schema and identify design smells, missing constraints, or performance hazards:

%s

Provide:
1. Architectural strengths and design quality evaluation.
2. High-priority schema risks or normalization smells.
3. Recommended improvements.`, schemaSummary)

	return c.callGemini(ctx, sys, prompt)
}

func (c *Client) Ask(ctx context.Context, question string, schemaContext string) (string, error) {
	sys := "You are SQL Doctor, an intelligent database diagnostics and engineering assistant. Ground your answer strictly in the provided database schema."
	prompt := fmt.Sprintf(`Question: %s

Connected Database Schema:
%s

Answer the user's question clearly and provide relevant SQL snippets or explanations based strictly on the provided schema.`, question, schemaContext)

	return c.callGemini(ctx, sys, prompt)
}

func (c *Client) GenerateSQL(ctx context.Context, userGoal string, schemaContext string) (*ai.GeneratedSQL, error) {
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

	raw, err := c.callGemini(ctx, sys, prompt)
	if err != nil {
		return nil, err
	}

	result := &ai.GeneratedSQL{}
	if strings.Contains(raw, "---SQL---") {
		parts := strings.Split(raw, "---SQL---")
		if len(parts) > 1 {
			subParts := strings.Split(parts[1], "---EXPLANATION---")
			result.SQL = cleanCodeBlock(subParts[0])
			if len(subParts) > 1 {
				result.Explanation = strings.TrimSpace(subParts[1])
			}
		}
	} else {
		result.SQL = cleanCodeBlock(raw)
	}

	upper := strings.ToUpper(result.SQL)
	if strings.Contains(upper, "DELETE") || strings.Contains(upper, "UPDATE") || strings.Contains(upper, "DROP") || strings.Contains(upper, "TRUNCATE") {
		result.IsDestructive = true
	}

	return result, nil
}

func (c *Client) SummarizeDoctor(ctx context.Context, doctorSummary string) (string, error) {
	sys := "You are a Database Reliability Engineer. Provide an executive summary of database health findings."
	prompt := fmt.Sprintf(`Summarize these database diagnostic findings into an executive briefing with prioritized action items:

%s`, doctorSummary)

	return c.callGemini(ctx, sys, prompt)
}

func cleanCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```sql") {
		s = strings.TrimPrefix(s, "```sql")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
