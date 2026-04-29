package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
	openai "github.com/openai/openai-go/v3"
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

func TestOpenAIReasoningEffort(t *testing.T) {
	prov := &OpenAIProvider{name: "openai-oauth"}

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
			params := prov.toChatParams(Request{Model: tt.model, Thinking: tt.thinking}, true)
			if got := string(params.ReasoningEffort); got != tt.want {
				t.Fatalf("expected reasoning_effort %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDeepSeekReasoningEffort(t *testing.T) {
	prov := &OpenAIProvider{name: "deepseek"}

	params := prov.toChatParams(Request{Model: "deepseek-v4-pro", Thinking: "xhigh"}, true)
	if got := string(params.ReasoningEffort); got != "high" {
		t.Fatalf("expected reasoning_effort high, got %q", got)
	}

	params = prov.toChatParams(Request{Model: "deepseek-v4-pro", Thinking: "none"}, true)
	if got := string(params.ReasoningEffort); got != "" {
		t.Fatalf("expected no reasoning_effort for none, got %q", got)
	}
}

func TestOpenAIReasoningEffortNone(t *testing.T) {
	prov := &OpenAIProvider{name: "openai-oauth"}

	params := prov.toChatParams(Request{Model: "gpt-5.1", Thinking: "none"}, true)
	if got := string(params.ReasoningEffort); got != "none" {
		t.Fatalf("expected reasoning_effort none, got %q", got)
	}
}

func TestExtractOpenRouterReasoningDelta(t *testing.T) {
	flat := "flat reasoning"
	text := "detail text"
	fallbackText := "fallback text"

	tests := []struct {
		name  string
		delta components.ChatStreamDelta
		want  string
	}{
		{
			name: "flat reasoning",
			delta: components.ChatStreamDelta{
				Reasoning: optionalnullable.From(&flat),
			},
			want: flat,
		},
		{
			name: "reasoning detail text",
			delta: components.ChatStreamDelta{
				ReasoningDetails: []components.ReasoningDetailUnion{
					components.CreateReasoningDetailUnionReasoningText(components.ReasoningDetailText{
						Text: optionalnullable.From(&text),
					}),
				},
			},
			want: text,
		},
		{
			name: "reasoning detail summary",
			delta: components.ChatStreamDelta{
				ReasoningDetails: []components.ReasoningDetailUnion{
					components.CreateReasoningDetailUnionReasoningSummary(components.ReasoningDetailSummary{
						Summary: "summary text",
					}),
				},
			},
			want: "summary text",
		},
		{
			name: "flat preferred over details",
			delta: components.ChatStreamDelta{
				Reasoning: optionalnullable.From(&flat),
				ReasoningDetails: []components.ReasoningDetailUnion{
					components.CreateReasoningDetailUnionReasoningText(components.ReasoningDetailText{
						Text: optionalnullable.From(&fallbackText),
					}),
				},
			},
			want: flat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractOpenRouterReasoningDelta(tt.delta); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestExtractOpenAIReasoningDeltaFromExtraFields(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "reasoning_content", raw: `{"reasoning_content":"deepseek thought"}`, want: "deepseek thought"},
		{name: "reasoning", raw: `{"reasoning":"openrouter thought"}`, want: "openrouter thought"},
		{name: "reasoning_text", raw: `{"reasoning_text":"compat thought"}`, want: "compat thought"},
		{name: "first non-empty alias", raw: `{"reasoning_content":"first","reasoning":"second"}`, want: "first"},
		{name: "ignore empty", raw: `{"reasoning_content":"","reasoning":"fallback"}`, want: "fallback"},
		{name: "none", raw: `{"content":"answer"}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var delta openai.ChatCompletionChunkChoiceDelta
			if err := json.Unmarshal([]byte(tt.raw), &delta); err != nil {
				t.Fatalf("unmarshal delta: %v", err)
			}
			if got := extractOpenAIReasoningDelta(delta); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
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
