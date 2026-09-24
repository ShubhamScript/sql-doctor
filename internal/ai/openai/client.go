package openai

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

// Client implements ai.AIProvider for OpenAI and OpenAI-compatible local APIs (Ollama, vLLM, etc.)
type Client struct {
	ai.BaseProvider
	apiKey   string
	endpoint string
	isOllama bool
	client   *http.Client
}

// Request payload for OpenAI Chat Completions API
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response structure for Chat Completions API
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// New creates an OpenAI or Ollama provider client
func New(apiKey, modelName, endpoint string, isOllama bool) *Client {
	providerName := "OpenAI"
	if isOllama {
		providerName = "Ollama (Local)"
		if endpoint == "" {
			endpoint = "http://localhost:11434/v1"
		}
		if modelName == "" {
			modelName = "deepseek-r1:8b"
		}
	} else {
		if endpoint == "" {
			endpoint = "https://api.openai.com/v1"
		}
		if modelName == "" {
			modelName = "gpt-4o-mini"
		}
	}

	endpoint = strings.TrimSuffix(endpoint, "/")

	configured := false
	if isOllama {
		// Ollama is configured if an endpoint is provided (keys are usually optional)
		configured = endpoint != ""
	} else {
		configured = strings.TrimSpace(apiKey) != ""
	}

	c := &Client{
		apiKey:   apiKey,
		endpoint: endpoint,
		isOllama: isOllama,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	c.BaseProvider = ai.BaseProvider{
		Name:       providerName,
		ModelName:  modelName,
		Configured: configured,
		Caller:     c.callOpenAI,
	}

	return c
}

func (c *Client) callOpenAI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !c.IsConfigured() {
		if c.isOllama {
			return "", fmt.Errorf("Ollama is not configured. Set endpoint using: sql-doctor config ai set-endpoint <url>")
		}
		return "", fmt.Errorf("OpenAI API key is not configured. Set key using: sql-doctor config ai set-key openai <key> or set OPENAI_API_KEY environment variable")
	}

	messages := make([]chatMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: userPrompt,
	})

	reqBody := chatRequest{
		Model:       c.ModelName,
		Messages:    messages,
		Temperature: 0.2,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	url := c.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s connection failed: %w", c.Name, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("%s returned non-JSON response (HTTP %d): %s", c.Name, resp.StatusCode, string(bodyBytes))
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", fmt.Errorf("%s API error: %s", c.Name, chatResp.Error.Message)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s HTTP error %d: %s", c.Name, resp.StatusCode, string(bodyBytes))
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("%s returned no completion choices", c.Name)
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}
