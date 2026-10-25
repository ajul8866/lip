package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxProviderBody = 4 << 20

type OpenAIProvider struct {
	base   string
	model  string
	apiKey string
	client *http.Client
}

func (p *OpenAIProvider) Kind() string { return "openai" }

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *openAIFormat   `json:"response_format,omitempty"`
}

type openAIFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, openAIMessage{Role: m.Role, Content: m.Content})
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	body := openAIRequest{
		Model:       p.model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
	}
	if req.JSON {
		body.ResponseFormat = &openAIFormat{Type: "json_object"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("User-Agent", "lip")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider request: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxProviderBody)
	data, err := io.ReadAll(limited)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatResponse{}, providerHTTPError("openai", resp.StatusCode, data)
	}
	var parsed openAIResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("openai provider response: %w", ErrBadOutput)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return ChatResponse{}, fmt.Errorf("openai provider: %s", safeMessage(parsed.Error.Message))
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return ChatResponse{}, fmt.Errorf("%w: empty completion", ErrBadOutput)
	}
	return ChatResponse{
		Content: parsed.Choices[0].Message.Content,
		Usage: Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
		},
	}, nil
}

func (p *OpenAIProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("User-Agent", "lip")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai provider ping: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openai provider ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

type OllamaProvider struct {
	base   string
	model  string
	client *http.Client
}

func (p *OllamaProvider) Kind() string { return "ollama" }

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error"`
}

func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		messages = append(messages, openAIMessage{Role: m.Role, Content: m.Content})
	}
	body := ollamaRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   false,
		Options:  map[string]any{"temperature": req.Temperature},
	}
	if req.JSON {
		body.Format = "json"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.base+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "lip")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("ollama read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatResponse{}, providerHTTPError("ollama", resp.StatusCode, data)
	}
	var parsed ollamaResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("ollama response: %w", ErrBadOutput)
	}
	if parsed.Error != "" {
		return ChatResponse{}, fmt.Errorf("ollama: %s", safeMessage(parsed.Error))
	}
	if strings.TrimSpace(parsed.Message.Content) == "" {
		return ChatResponse{}, fmt.Errorf("%w: empty completion", ErrBadOutput)
	}
	return ChatResponse{Content: parsed.Message.Content}, nil
}

func (p *OllamaProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/api/tags", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "lip")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama ping: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

func providerHTTPError(name string, status int, data []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &payload)
	msg := safeMessage(payload.Error.Message)
	if msg == "" {
		msg = "request failed"
	}
	return fmt.Errorf("%s provider: HTTP %d: %s", name, status, msg)
}

func safeMessage(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
