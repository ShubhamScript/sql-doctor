package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sql-doctor/sql-doctor/internal/ai"
)

// Client implements ai.AIProvider for Anthropic Claude via the Messages API
type Client struct {
	ai.BaseProvider
	apiKey string
	client *http.Client
}

type messagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	System      string           `json:"system,omitempty"`
	Messages    []messagePayload `json:"messages"`
	Temperature float64          `json:"temperature"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// New creates an Anthropic Claude provider client
func New(apiKey, modelName string) *Client {
	if modelName == "" {
		modelName = "claude-3-5-haiku-20241022"
	}

	c := &Client{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	c.BaseProvider = ai.BaseProvider{
		Name:       "Anthropic Claude",
		ModelName:  modelName,
		Configured: strings.TrimSpace(apiKey) != "",
		Caller:     c.callClaude,
	}

	return c
}

func (c *Client) callClaude(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !c.IsConfigured() {
		return "", fmt.Errorf("Anthropic Claude API key is not configured. Set key using: sql-doctor config ai set-key claude <key> or set ANTHROPIC_API_KEY environment variable")
	}

	reqBody := claudeRequest{
		Model:     c.ModelName,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []messagePayload{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Claude request: %w", err)
	}

	url := "https://api.anthropic.com/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic Claude connection failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var clResp claudeResponse
	if err := json.Unmarshal(bodyBytes, &clResp); err != nil {
		return "", fmt.Errorf("Anthropic Claude returned non-JSON response (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	if clResp.Error != nil && clResp.Error.Message != "" {
		return "", fmt.Errorf("Anthropic Claude API error: %s", clResp.Error.Message)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Anthropic Claude HTTP error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var sb strings.Builder
	for _, block := range clResp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}

	return strings.TrimSpace(sb.String()), nil
}
