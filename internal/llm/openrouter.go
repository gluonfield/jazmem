package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	APIKey  string
	Model   string
	BaseURL string
}

type Client struct {
	cfg  Config
	http *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Messages  []Message
	MaxTokens int
}

type Response struct {
	Content string
	Model   string
}

func New(cfg Config) *Client {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (c *Client) CompleteJSON(ctx context.Context, req Request) (Response, error) {
	if c == nil {
		return Response{}, errors.New("OpenRouter client is not configured")
	}
	if c.cfg.APIKey == "" {
		return Response{}, errors.New("OPENROUTER_API_KEY is required for this jazmem command")
	}
	if c.cfg.Model == "" {
		return Response{}, errors.New("OPENROUTER_MODEL is required for this jazmem command")
	}
	if c.cfg.BaseURL == "" {
		return Response{}, errors.New("OpenRouter base URL is empty")
	}
	if len(req.Messages) == 0 {
		return Response{}, errors.New("LLM messages are empty")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1800
	}
	payload := map[string]any{
		"model":       c.cfg.Model,
		"messages":    req.Messages,
		"temperature": 0.1,
		"max_tokens":  maxTokens,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Title", "jazmem")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		text := strings.TrimSpace(string(body))
		if len(text) > 1000 {
			text = text[:1000]
		}
		return Response{}, fmt.Errorf("OpenRouter request failed: status %d: %s", resp.StatusCode, text)
	}

	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Response{}, fmt.Errorf("decode OpenRouter response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return Response{}, errors.New("OpenRouter returned no message content")
	}
	model := strings.TrimSpace(parsed.Model)
	if model == "" {
		model = c.cfg.Model
	}
	return Response{Content: ExtractJSONObject(parsed.Choices[0].Message.Content), Model: model}, nil
}

func ExtractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 3 {
			lines = lines[1:]
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			content = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(content[start : end+1])
	}
	return content
}
