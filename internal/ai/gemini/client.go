package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/sql-doctor/sql-doctor/internal/ai"
	"google.golang.org/api/option"
)

// Client implements ai.AIProvider via the official Google Gemini Go SDK
type Client struct {
	ai.BaseProvider
	apiKey string
}

func New(apiKey, modelName string) *Client {
	if modelName == "" {
		modelName = "gemini-3.8-flash"
	}
	c := &Client{
		apiKey: apiKey,
	}
	c.BaseProvider = ai.BaseProvider{
		Name:       "Google Gemini",
		ModelName:  modelName,
		Configured: strings.TrimSpace(apiKey) != "",
		Caller:     c.callGemini,
	}
	return c
}

func (c *Client) callGemini(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !c.IsConfigured() {
		return "", fmt.Errorf("AI features are unavailable because a Gemini API key has not been configured.\n\nConfigure your key using:\n  sql-doctor config ai set-key gemini <your-key>\nor set the GEMINI_API_KEY environment variable")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to initialize Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(c.ModelName)
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
