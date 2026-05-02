package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
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
	if _, ok := prov.(*OpenAIProvider); !ok {
		t.Fatalf("expected OpenRouter to use OpenAI-compatible provider, got %T", prov)
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

func TestCreateAnthropic(t *testing.T) {
	prov, err := Create("anthropic", "sk-ant-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "anthropic" {
		t.Errorf("expected name anthropic, got %s", prov.Name())
	}
}

func TestCreateOpenAIOAuth(t *testing.T) {
	prov, err := Create("openai-codex", testOpenAICodexToken("acct_test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "openai-codex" {
		t.Errorf("expected name openai-codex, got %s", prov.Name())
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
	prov, err := NewOpenAICodex(testOpenAICodexToken("acct_test"))
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
	if models[0] != "gpt-5.5" {
		t.Fatalf("expected gpt-5.5 first, got %q", models[0])
	}
	for _, unsupported := range []string{"gpt-5.1", "gpt-5.3"} {
		for _, model := range models {
			if model == unsupported {
				t.Fatalf("unsupported Codex model %q should not be listed: %#v", unsupported, models)
			}
		}
	}
}

func TestOpenAICodexStreamUsesChatGPTBackendHeadersAndResponsesBody(t *testing.T) {
	token := testOpenAICodexToken("acct_test")
	var gotHeaders http.Header
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		if r.URL.Path != "/codex/responses" {
			t.Fatalf("expected /codex/responses, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":2}}}}\n\n"))
	}))
	defer server.Close()

	prov, err := NewOpenAICodex(token)
	if err != nil {
		t.Fatalf("NewOpenAICodex: %v", err)
	}
	codex := prov.(*OpenAICodexProvider)
	codex.baseURL = server.URL + "/codex/responses"

	stream, err := codex.Stream(context.Background(), Request{Model: "gpt-5.5", Thinking: "minimal", SystemPrompt: "sys", Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var text string
	var usage *Usage
	for ev := range stream {
		if ev.Error != nil {
			t.Fatalf("stream error: %v", ev.Error)
		}
		text += ev.TextDelta
		if ev.Done != nil {
			usage = ev.Done
		}
	}
	if text != "Hello" {
		t.Fatalf("expected Hello, got %q", text)
	}
	if usage == nil || usage.InputTokens != 5 || usage.OutputTokens != 3 || usage.CacheReadTokens != 2 || !usage.Subscription {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if gotHeaders.Get("Authorization") != "Bearer "+token {
		t.Fatalf("missing bearer auth: %s", gotHeaders.Get("Authorization"))
	}
	if gotHeaders.Get("chatgpt-account-id") != "acct_test" {
		t.Fatalf("missing account id header: %s", gotHeaders.Get("chatgpt-account-id"))
	}
	if gotHeaders.Get("OpenAI-Beta") != "responses=experimental" || gotHeaders.Get("Accept") != "text/event-stream" {
		t.Fatalf("unexpected codex headers: %#v", gotHeaders)
	}
	if gotBody["model"] != "gpt-5.5" || gotBody["instructions"] != "sys" || gotBody["store"] != false || gotBody["stream"] != true {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
	if reasoning, _ := gotBody["reasoning"].(map[string]any); reasoning["effort"] != "low" || reasoning["summary"] != "auto" {
		t.Fatalf("unexpected reasoning: %#v", gotBody["reasoning"])
	}
}

func TestOpenAICodexRejectsTokenWithoutAccountID(t *testing.T) {
	if _, err := NewOpenAICodex("not-a-jwt"); err == nil {
		t.Fatal("expected invalid OAuth token error")
	}
}

func testOpenAICodexToken(accountID string) string {
	payload := map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID}}
	data, _ := json.Marshal(payload)
	return "aaa." + base64.RawURLEncoding.EncodeToString(data) + ".bbb"
}

func TestOpenAIReasoningEffort(t *testing.T) {
	prov := &OpenAIProvider{name: "openai-codex"}

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
	if got := string(params.ReasoningEffort); got != "max" {
		t.Fatalf("expected reasoning_effort max, got %q", got)
	}

	params = prov.toChatParams(Request{Model: "deepseek-v4-pro", Thinking: "medium"}, true)
	if got := string(params.ReasoningEffort); got != "high" {
		t.Fatalf("expected reasoning_effort high, got %q", got)
	}

	params = prov.toChatParams(Request{Model: "deepseek-v4-pro", Thinking: "none"}, true)
	if got := string(params.ReasoningEffort); got != "" {
		t.Fatalf("expected no reasoning_effort for none, got %q", got)
	}
}

func TestDeepSeekThinkingToggle(t *testing.T) {
	prov := &OpenAIProvider{name: "deepseek"}

	params := prov.toChatParams(Request{Model: "deepseek-v4-pro", Thinking: "none"}, true)
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	thinking, ok := raw["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("expected disabled thinking toggle, got %s", string(data))
	}

	params = prov.toChatParams(Request{Model: "deepseek-v4-pro", Thinking: "high"}, true)
	data, err = json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	thinking, ok = raw["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("expected enabled thinking toggle, got %s", string(data))
	}
}

func TestDeepSeekAssistantToolCallIncludesEmptyReasoningContent(t *testing.T) {
	prov := &OpenAIProvider{name: "deepseek"}
	params := prov.toChatParams(Request{
		Model:    "deepseek-v4-pro",
		Thinking: "high",
		Messages: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{{Name: "echo", ID: "call_1", Args: map[string]any{"message": "hi"}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "hi"},
		},
	}, true)

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var raw struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(raw.Messages) == 0 {
		t.Fatalf("expected messages, got %s", string(data))
	}
	reasoning, ok := raw.Messages[0]["reasoning_content"]
	if !ok {
		t.Fatalf("expected reasoning_content field on assistant tool-call message, got %s", string(data))
	}
	if reasoning != "" {
		t.Fatalf("expected empty reasoning_content to be preserved as empty string, got %#v", reasoning)
	}
}

func TestOpenAIReasoningEffortNone(t *testing.T) {
	prov := &OpenAIProvider{name: "openai-codex"}

	params := prov.toChatParams(Request{Model: "gpt-5.1", Thinking: "none"}, true)
	if got := string(params.ReasoningEffort); got != "none" {
		t.Fatalf("expected reasoning_effort none, got %q", got)
	}
}

func TestAnthropicMessageParams(t *testing.T) {
	prov := &AnthropicProvider{}
	params := prov.toMessageParams(Request{
		Model:        "claude-sonnet-4-5",
		Thinking:     "medium",
		SystemPrompt: "system prompt",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "I'll call a tool", ToolCalls: []ToolCall{{Name: "echo", ID: "toolu_1", Args: map[string]any{"message": "hi"}}}},
			{Role: "tool", ToolCallID: "toolu_1", Content: "hi"},
		},
		Tools: []ToolDefinition{{Name: "echo", Description: "Echo", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}}}}},
	})

	if params.Model != anthropic.Model("claude-sonnet-4-5") {
		t.Fatalf("unexpected model: %s", params.Model)
	}
	if len(params.System) != 1 || params.System[0].Text != "system prompt" {
		t.Fatalf("unexpected system prompt: %#v", params.System)
	}
	if len(params.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %#v", params.Messages)
	}
	if params.Messages[1].Role != anthropic.MessageParamRoleAssistant || len(params.Messages[1].Content) != 2 {
		t.Fatalf("expected assistant text plus tool_use blocks, got %#v", params.Messages[1])
	}
	if params.Messages[2].Role != anthropic.MessageParamRoleUser || len(params.Messages[2].Content) != 1 || params.Messages[2].Content[0].OfToolResult == nil {
		t.Fatalf("expected tool result user message, got %#v", params.Messages[2])
	}
	if len(params.Tools) != 1 || params.Tools[0].OfTool == nil || params.Tools[0].OfTool.Name != "echo" {
		t.Fatalf("unexpected tools: %#v", params.Tools)
	}
	if params.Thinking.OfEnabled == nil || params.Thinking.OfEnabled.BudgetTokens != 2048 {
		t.Fatalf("expected medium thinking budget, got %#v", params.Thinking)
	}
}

func TestAnthropicThinking(t *testing.T) {
	tests := []struct {
		thinking string
		wantNil  bool
		want     int64
	}{
		{thinking: "none", wantNil: true},
		{thinking: "", wantNil: true},
		{thinking: "minimal", want: 1024},
		{thinking: "low", want: 1024},
		{thinking: "medium", want: 2048},
		{thinking: "high", want: 4096},
		{thinking: "xhigh", want: 6144},
	}
	for _, tt := range tests {
		t.Run(tt.thinking, func(t *testing.T) {
			got := anthropicThinking(tt.thinking)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil thinking config, got %#v", got)
				}
				return
			}
			if got == nil || got.OfEnabled == nil || got.OfEnabled.BudgetTokens != tt.want {
				t.Fatalf("expected budget %d, got %#v", tt.want, got)
			}
		})
	}
}

func TestAnthropicToolSchemaMapping(t *testing.T) {
	tools := anthropicTools([]ToolDefinition{
		{
			Name:        "search",
			Description: "Search for documents",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"query": map[string]any{"type": "string", "description": "search query"}},
				"required":             []any{"query"},
				"additionalProperties": false,
			},
		},
	})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].OfTool
	if tool == nil {
		t.Fatal("expected OfTool to be non-nil")
	}
	if tool.InputSchema.Properties == nil {
		t.Fatal("expected Properties to be set from schema map")
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "query" {
		t.Fatalf("expected Required to be [query], got %v", tool.InputSchema.Required)
	}
	if _, ok := tool.InputSchema.ExtraFields["additionalProperties"]; !ok {
		t.Fatal("expected ExtraFields to contain additionalProperties")
	}
	if _, ok := tool.InputSchema.ExtraFields["properties"]; ok {
		t.Fatal("properties should not be in ExtraFields")
	}
	if _, ok := tool.InputSchema.ExtraFields["type"]; ok {
		t.Fatal("type should not be in ExtraFields")
	}
}

func TestOpenRouterReasoningUsesOpenAICompatibleParam(t *testing.T) {
	prov := &OpenAIProvider{name: "openrouter"}
	params := prov.toChatParams(Request{Model: "deepseek/deepseek-r1", Thinking: "medium"}, true)

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	reasoning, ok := raw["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object, got %s", string(data))
	}
	if reasoning["effort"] != "medium" {
		t.Fatalf("expected reasoning effort medium, got %#v", reasoning["effort"])
	}
	if _, ok := raw["reasoning_effort"]; ok {
		t.Fatalf("expected no OpenAI reasoning_effort for OpenRouter, got %s", string(data))
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
		{name: "reasoning detail text", raw: `{"reasoning_details":[{"type":"reasoning.text","text":"detail thought"}]}`, want: "detail thought"},
		{name: "reasoning detail summary", raw: `{"reasoning_details":[{"type":"reasoning.summary","summary":"summary thought"}]}`, want: "summary thought"},
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
