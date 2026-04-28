package provider

import (
	"context"
	"errors"
	"testing"
)

func TestCreateOpenRouter(t *testing.T) {
	prov, err := Create("openrouter", "sk-or-v1-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "openrouter" {
		t.Errorf("expected name openrouter, got %s", prov.Name())
	}
}

func TestCreateOpenAI(t *testing.T) {
	prov, err := Create("openai", "sk-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "openai" {
		t.Errorf("expected name openai, got %s", prov.Name())
	}
}

func TestCreateDeepSeek(t *testing.T) {
	prov, err := Create("deepseek", "sk-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "deepseek" {
		t.Errorf("expected name deepseek, got %s", prov.Name())
	}
}

func TestCreateOpenAIOAuth(t *testing.T) {
	prov, err := Create("openai-oauth", "oauth-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "openai-oauth" {
		t.Errorf("expected name openai-oauth, got %s", prov.Name())
	}
}

func TestCreateUnsupported(t *testing.T) {
	prov, err := Create("nonexistent", "key")
	if err == nil {
		t.Fatal("expected error")
	}
	if prov != nil {
		t.Errorf("expected nil provider, got %v", prov)
	}
	if err.Error() != "unsupported provider: nonexistent" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestErrUnsupportedProvider(t *testing.T) {
	err := ErrUnsupportedProvider("foo")
	if err.Error() != "unsupported provider: foo" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestErrStreamClosed(t *testing.T) {
	if ErrStreamClosed.Error() != "stream closed" {
		t.Errorf("unexpected error: %v", ErrStreamClosed)
	}
}

func TestMessageToolCallID(t *testing.T) {
	m := Message{Role: "tool", ToolCallID: "call_abc123", Content: "some result"}
	if m.ToolCallID != "call_abc123" {
		t.Errorf("expected call_abc123, got %s", m.ToolCallID)
	}
}

func TestDeepSeekListModelsUsesStaticModels(t *testing.T) {
	prov, err := NewDeepSeek("sk-test")
	if err != nil {
		t.Fatalf("NewDeepSeek: %v", err)
	}
	models, err := prov.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0] != "deepseek-v4-pro" || models[1] != "deepseek-v4-flash" {
		t.Fatalf("unexpected deepseek models: %#v", models)
	}
}

func TestOpenAIOAuthListModelsUsesStaticFallback(t *testing.T) {
	prov, err := NewOpenAI("oauth.jwt.token")
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	models, err := prov.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected static OAuth models")
	}
	if models[0] != "gpt-5.1" {
		t.Fatalf("expected gpt-5.1 first, got %q", models[0])
	}
	if models[len(models)-1] != "gpt-5.5" {
		t.Fatalf("expected gpt-5.5 last, got %q", models[len(models)-1])
	}
}

func TestOpenAIRequestMapsThinkingEffortLikePiCodex(t *testing.T) {
	prov := &OpenAIProvider{name: "openai-oauth", apiKey: "oauth-token"}

	tests := []struct {
		model    string
		thinking string
		want     string
	}{
		{model: "gpt-5.1", thinking: "xhigh", want: "high"},
		{model: "gpt-5.2", thinking: "minimal", want: "low"},
		{model: "gpt-5.5", thinking: "medium", want: "medium"},
		{model: "gpt-5.1-codex-mini", thinking: "low", want: "medium"},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.thinking, func(t *testing.T) {
			body := prov.buildChatRequest(Request{Model: tt.model, Thinking: tt.thinking}, true)
			if got := body["reasoning_effort"]; got != tt.want {
				t.Fatalf("expected reasoning_effort %q, got %#v", tt.want, got)
			}
		})
	}
}

func TestDeepSeekRequestMapsThinkingEffort(t *testing.T) {
	prov := &OpenAIProvider{name: "deepseek", apiKey: "sk-test", baseURL: deepSeekBaseURL}
	body := prov.buildChatRequest(Request{Model: "deepseek-v4-pro", Thinking: "xhigh"}, true)
	if got := body["reasoning_effort"]; got != "high" {
		t.Fatalf("expected reasoning_effort high, got %#v", got)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("expected thinking enabled, got %#v", body["thinking"])
	}

	body = prov.buildChatRequest(Request{Model: "deepseek-v4-pro", Thinking: "none"}, true)
	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("expected no reasoning_effort for none, got %#v", body["reasoning_effort"])
	}
	if _, ok := body["thinking"]; ok {
		t.Fatalf("expected no thinking for none, got %#v", body["thinking"])
	}
}

func TestOpenAIRequestSendsNoneThinkingEffort(t *testing.T) {
	prov := &OpenAIProvider{name: "openai-oauth", apiKey: "oauth-token"}
	body := prov.buildChatRequest(Request{Model: "gpt-5.1", Thinking: "none"}, true)
	if got := body["reasoning_effort"]; got != "none" {
		t.Fatalf("expected reasoning_effort none, got %#v", got)
	}
}

func TestModelRegistryLoadModelsClearsAndReportsErrors(t *testing.T) {
	reg := NewModelRegistry()
	reg.AddProvider(fakeProvider{name: "ok", models: []string{"m1"}})
	if err := reg.LoadModels(context.Background()); err != nil {
		t.Fatalf("LoadModels: %v", err)
	}
	if got := reg.GetAll(); len(got) != 1 || got[0].ID != "m1" {
		t.Fatalf("expected one model, got %#v", got)
	}

	reg.AddProvider(fakeProvider{name: "ok", models: []string{"m2"}})
	reg.AddProvider(fakeProvider{name: "bad", err: errors.New("boom")})
	if err := reg.LoadModels(context.Background()); err == nil {
		t.Fatal("expected load error")
	}
	got := reg.GetAll()
	if len(got) != 1 || got[0].ID != "m2" {
		t.Fatalf("expected reloaded m2 only, got %#v", got)
	}
}

func TestParseToolArgs(t *testing.T) {
	// Valid JSON.
	args := parseToolArgs(`{"key": "value", "num": 42}`)
	if args["key"] != "value" {
		t.Errorf("expected value, got %v", args["key"])
	}
	if args["num"] != float64(42) {
		t.Errorf("expected 42, got %v", args["num"])
	}

	// Empty string.
	if parsed := parseToolArgs(""); parsed != nil {
		t.Errorf("expected nil, got %v", parsed)
	}

	// Invalid JSON.
	parsed := parseToolArgs("not json")
	if parsed["raw"] != "not json" {
		t.Errorf("expected raw fallback, got %v", parsed)
	}
}

type fakeProvider struct {
	name   string
	models []string
	err    error
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Send(context.Context, Request) (*Response, error) {
	return nil, nil
}
func (f fakeProvider) Stream(context.Context, Request) (<-chan StreamEvent, error) {
	return nil, nil
}
func (f fakeProvider) ListModels(context.Context) ([]string, error) {
	return f.models, f.err
}
