package gemini

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

// Client implements ai.AIProvider for Google Gemini via the official REST API
type Client struct {
	ai.BaseProvider
	apiKey   string
	endpoint string
	client   *http.Client
}

// Request structure for Gemini GenerateContent REST API
type geminiRequest struct {
	SystemInstruction *geminiContent   `json:"system_instruction,omitempty"`
	Contents          []geminiContent  `json:"contents"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiGenConfig struct {
	Temperature float64 `json:"temperature,omitempty"`
}

// Response structure for Gemini GenerateContent REST API
type geminiResponse struct {
	Candidates []struct {
		Content *struct {
			Parts []geminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// New creates a Google Gemini provider client with default Google API endpoint
func New(apiKey, modelName string) *Client {
	return NewWithEndpoint(apiKey, modelName, "")
}

// NewWithEndpoint creates a Google Gemini provider client with a custom endpoint
func NewWithEndpoint(apiKey, modelName, endpoint string) *Client {
	if modelName == "" {
		modelName = "gemini-3.8-flash"
	}
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com"
	}
	c := &Client{
		apiKey:   apiKey,
		endpoint: strings.TrimRight(endpoint, "/"),
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
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

	cleanModel := strings.TrimPrefix(strings.TrimSpace(c.ModelName), "models/")
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.endpoint, cleanModel)

	reqPayload := geminiRequest{
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{Text: userPrompt},
				},
			},
		},
		GenerationConfig: &geminiGenConfig{
			Temperature: 0.2,
		},
	}

	if systemPrompt != "" {
		reqPayload.SystemInstruction = &geminiContent{
			Parts: []geminiPart{
				{Text: systemPrompt},
			},
		}
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to encode request payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request timed out or was cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("network connection error calling Google Gemini: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse Google Gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if geminiResp.Error != nil && geminiResp.Error.Message != "" {
			return "", fmt.Errorf("Google Gemini API error (status %d): %s", resp.StatusCode, geminiResp.Error.Message)
		}
		return "", fmt.Errorf("Google Gemini API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if geminiResp.Error != nil && geminiResp.Error.Message != "" {
		return "", fmt.Errorf("Google Gemini API error: %s", geminiResp.Error.Message)
	}

	var responseText strings.Builder
	var thoughtText strings.Builder

	for _, cand := range geminiResp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Thought {
					thoughtText.WriteString(part.Text)
				} else if part.Text != "" {
					responseText.WriteString(part.Text)
				}
			}
		}
	}

	final := strings.TrimSpace(responseText.String())
	if final == "" {
		// Fallback to thoughts if the model reasoned without normal parts
		final = strings.TrimSpace(thoughtText.String())
	}

	if final == "" {
		if geminiResp.PromptFeedback != nil && geminiResp.PromptFeedback.BlockReason != "" {
			return "", fmt.Errorf("Google Gemini blocked prompt (reason: %s)", geminiResp.PromptFeedback.BlockReason)
		}
		if len(geminiResp.Candidates) > 0 && geminiResp.Candidates[0].FinishReason != "" && geminiResp.Candidates[0].FinishReason != "STOP" {
			return "", fmt.Errorf("Google Gemini stopped generating (finishReason: %s)", geminiResp.Candidates[0].FinishReason)
		}
		return "", fmt.Errorf("received empty response from Google Gemini")
	}

	return final, nil
}
