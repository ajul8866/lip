package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/lip-project/lip/internal/egress"
	"github.com/lip-project/lip/internal/store"
)

func TestParsePlanBounds(t *testing.T) {
	valid := `{"tasks":[{"title":"Check assets","description":"Review inventory","tool":"inventory_summary","parameters":{}}]}`
	tasks, err := parsePlan(valid)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("valid plan rejected: %v", err)
	}

	invalid := []string{
		`{"tasks":[]}`,
		`{"tasks":[{"title":"x","description":"","tool":"unknown","parameters":{}}]}`,
		`{"tasks":[{"title":"x","description":"","tool":"http_head","parameters":{"url":"ftp://example.com"}}]}`,
		`{"tasks":[{"title":"x","description":"","tool":"http_head","parameters":{"url":"https://user@example.com"}}]}`,
		`not json`,
	}
	for _, raw := range invalid {
		if _, err := parsePlan(raw); !errors.Is(err, ErrBadOutput) {
			t.Errorf("expected ErrBadOutput for %s, got %v", raw, err)
		}
	}
}

func TestParseFindingsRejectsInvalidSeverity(t *testing.T) {
	_, err := parseFindings(`{"findings":[{"title":"x","description":"y","severity":"urgent","evidence":{}}]}`)
	if !errors.Is(err, ErrBadOutput) {
		t.Fatalf("expected bad output, got %v", err)
	}
	findings, err := parseFindings("```json\n{\"findings\":[{\"title\":\"Review\",\"description\":\"Draft\",\"severity\":\"medium\",\"evidence\":{\"asset\":\"example.com\"}}]}\n```")
	if err != nil || len(findings) != 1 {
		t.Fatalf("valid finding rejected: %v", err)
	}
}

func TestRegistrySafeModeProviderPolicy(t *testing.T) {
	guard, err := egress.New(egress.Options{Safe: true, Allowlist: []string{"127.0.0.1:11434"}})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(guard)
	ctx := context.Background()

	if err := registry.Validate(ctx, store.LLMProvider{
		Kind: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt", APIKeyEnv: "LIP_LLM_API_KEY",
	}); !errors.Is(err, ErrProviderDenied) {
		t.Fatalf("cloud provider should be denied in safe mode: %v", err)
	}
	if err := registry.Validate(ctx, store.LLMProvider{
		Kind: "ollama", BaseURL: "http://192.0.2.10:11434", Model: "llama",
	}); !errors.Is(err, ErrProviderDenied) {
		t.Fatalf("non-allowlisted ollama should be denied: %v", err)
	}
	if err := registry.Validate(ctx, store.LLMProvider{
		Kind: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "llama",
	}); err != nil {
		t.Fatalf("allowlisted ollama should validate: %v", err)
	}
}

func TestExtractJSON(t *testing.T) {
	if got := extractJSON("prefix ```json\n{\"a\":1}\n``` suffix"); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
	if got := extractJSON("nothing"); got != "" {
		t.Fatalf("got %q", got)
	}
}
