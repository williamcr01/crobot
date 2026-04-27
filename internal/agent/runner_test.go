package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"crobot/internal/config"
	"crobot/internal/provider"
	"crobot/internal/tools"
)

// mockProvider simulates an LLM provider for testing.
type mockProvider struct {
	name      string
	responses []responseStep
	stepIdx   int
}

type responseStep struct {
	text      string
	toolCalls []provider.ToolCall
	usage     *provider.Usage
	err       error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Send(ctx context.Context, req provider.Request) (*provider.Response, error) {
	// Not used in the current test setup; stream is used instead.
	return nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		defer close(ch)
		step := m.responses[m.stepIdx]
		m.stepIdx++

		// Stream text.
		if step.text != "" {
			for _, r := range step.text {
				ch <- provider.StreamEvent{TextDelta: string(r)}
				time.Sleep(time.Microsecond)
			}
		}

		// Stream tool calls.
		for _, tc := range step.toolCalls {
			ch <- provider.StreamEvent{ToolCallStart: &provider.ToolCall{Name: tc.Name, ID: tc.ID, Args: nil}}
			if len(tc.Args) > 0 {
				ch <- provider.StreamEvent{ToolCallArgsDelta: `{"dummy": true}`}
			}
			ch <- provider.StreamEvent{ToolCallEnd: &provider.ToolCall{
				Name: tc.Name,
				ID:   tc.ID,
				Args: tc.Args,
			}}
		}

		if step.usage != nil {
			ch <- provider.StreamEvent{Done: step.usage}
		}
	}()
	return ch, nil
}

func (m *mockProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func TestRun_SimpleText(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{text: "Hello!", usage: &provider.Usage{InputTokens: 10, OutputTokens: 2}},
		},
	}

	cfg := &config.AgentConfig{
		Model:        "mock-model",
		SystemPrompt: "You are a test assistant.",
		MaxSteps:     5,
		MaxCost:      1.0,
	}

	toolReg := tools.NewRegistry()
	var events []Event
	onEvent := func(ev Event) {
		events = append(events, ev)
	}

	result, err := Run(context.Background(), mock, cfg, nil, toolReg, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", result.Text)
	}
	if result.Usage == nil {
		t.Fatal("expected usage")
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 2 {
		t.Errorf("unexpected usage: %+v", result.Usage)
	}
	// Should have text events.
	if len(events) == 0 {
		t.Error("expected events")
	}
}

func TestRun_ToolCall(t *testing.T) {
	// Register a simple tool.
	echoTool := tools.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			return map[string]any{"echo": msg}, nil
		},
	}

	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{
				text: "",
				toolCalls: []provider.ToolCall{
					{Name: "echo", ID: "call_1", Args: map[string]any{"message": "testing"}},
				},
				usage: &provider.Usage{InputTokens: 10, OutputTokens: 5},
			},
			{
				text:  "Done!",
				usage: &provider.Usage{InputTokens: 20, OutputTokens: 3},
			},
		},
	}

	cfg := &config.AgentConfig{
		Model:        "mock-model",
		SystemPrompt: "You are a test assistant.",
		MaxSteps:     5,
		MaxCost:      1.0,
	}

	toolReg := tools.NewRegistry()
	toolReg.Register(echoTool)
	var events []Event
	onEvent := func(ev Event) {
		events = append(events, ev)
	}

	result, err := Run(context.Background(), mock, cfg, nil, toolReg, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Done!" {
		t.Errorf("expected 'Done!', got %q", result.Text)
	}
	// Should have tool call events.
	hasToolCall := false
	hasToolResult := false
	for _, ev := range events {
		if ev.ToolCall != nil && ev.ToolCall.Name == "echo" && !ev.ToolCall.Start {
			hasToolCall = true
		}
		if ev.ToolResult != nil && ev.ToolResult.Name == "echo" {
			hasToolResult = true
		}
	}
	if !hasToolCall {
		t.Error("expected tool call end event")
	}
	if !hasToolResult {
		t.Error("expected tool result event")
	}
}

func TestRun_MaxStepsReached(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{text: "Step 1", toolCalls: []provider.ToolCall{
				{Name: "echo", ID: "call_1", Args: map[string]any{"message": "test"}},
			}, usage: &provider.Usage{InputTokens: 5, OutputTokens: 5}},
		},
	}

	echoTool := tools.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			return map[string]any{"echo": msg}, nil
		},
	}

	cfg := &config.AgentConfig{
		Model:        "mock-model",
		SystemPrompt: "You are a test assistant.",
		MaxSteps:     1,
		MaxCost:      1.0,
	}

	toolReg := tools.NewRegistry()
	toolReg.Register(echoTool)

	_, err := Run(context.Background(), mock, cfg, nil, toolReg, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MaxSteps=1 gave one step with a tool call, then the tool result goes into the next iteration
	// but the loop should complete because the mock only has one responseStep defined.
	// Actually with MaxSteps=1, on the second iteration it hits the limit and returns results.
	// This test verifies it doesn't hang or crash.
}

func TestRun_Cancellation(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{text: "before cancellation..."},
		},
	}

	cfg := &config.AgentConfig{
		Model:        "mock-model",
		SystemPrompt: "You are a test assistant.",
		MaxSteps:     10,
		MaxCost:      1.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := Run(ctx, mock, cfg, nil, tools.NewRegistry(), nil, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestRun_PluginHooks(t *testing.T) {
	// Minimal test that verifies PluginManager interface is called.
	type hookCall struct {
		name string
	}
	var hookCalls []hookCall

	pluginManager := &mockPluginManager{
		onPrePrompt: func(prompt string, msgs []provider.Message) (string, []provider.Message, error) {
			hookCalls = append(hookCalls, hookCall{"pre_prompt"})
			return prompt + " (modified by plugin)", msgs, nil
		},
		onPostResponse: func(resp *Result) (*Result, error) {
			hookCalls = append(hookCalls, hookCall{"post_response"})
			return resp, nil
		},
		onPreToolCall: func(name string, args map[string]any) (string, map[string]any, bool, error) {
			hookCalls = append(hookCalls, hookCall{"pre_tool"})
			return name, args, false, nil
		},
		onPostToolResult: func(name string, args map[string]any, result any) (any, error) {
			hookCalls = append(hookCalls, hookCall{"post_tool"})
			return result, nil
		},
		onOnEvent: func(ev any) error {
			hookCalls = append(hookCalls, hookCall{"on_event"})
			return nil
		},
	}

	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{
				text: "Hello from the plugin test!",
				usage: &provider.Usage{InputTokens: 10, OutputTokens: 2},
			},
		},
	}

	cfg := &config.AgentConfig{
		Model:        "mock-model",
		SystemPrompt: "You are a test assistant.",
		MaxSteps:     10,
		MaxCost:      1.0,
	}

	_, err := Run(context.Background(), mock, cfg, nil, tools.NewRegistry(), pluginManager, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have at least pre_prompt and post_response.
	foundPrePrompt := false
	foundPostResponse := false
	for _, c := range hookCalls {
		if c.name == "pre_prompt" {
			foundPrePrompt = true
		}
		if c.name == "post_response" {
			foundPostResponse = true
		}
	}
	if !foundPrePrompt {
		t.Error("expected pre_prompt hook call")
	}
	if !foundPostResponse {
		t.Error("expected post_response hook call")
	}
}

// mockPluginManager implements PluginManager for testing.
type mockPluginManager struct {
	onPrePrompt     func(string, []provider.Message) (string, []provider.Message, error)
	onPostResponse  func(*Result) (*Result, error)
	onPreToolCall   func(string, map[string]any) (string, map[string]any, bool, error)
	onPostToolResult func(string, map[string]any, any) (any, error)
	onOnEvent       func(any) error
}

func (m *mockPluginManager) CallPrePrompt(prompt string, msgs []provider.Message) (string, []provider.Message, error) {
	if m.onPrePrompt != nil {
		return m.onPrePrompt(prompt, msgs)
	}
	return prompt, msgs, nil
}

func (m *mockPluginManager) CallPostResponse(resp *Result) (*Result, error) {
	if m.onPostResponse != nil {
		return m.onPostResponse(resp)
	}
	return resp, nil
}

func (m *mockPluginManager) CallPreToolCall(name string, args map[string]any) (string, map[string]any, bool, error) {
	if m.onPreToolCall != nil {
		return m.onPreToolCall(name, args)
	}
	return name, args, false, nil
}

func (m *mockPluginManager) CallPostToolResult(name string, args map[string]any, result any) (any, error) {
	if m.onPostToolResult != nil {
		return m.onPostToolResult(name, args, result)
	}
	return result, nil
}

func (m *mockPluginManager) CallOnEvent(ev any) error {
	if m.onOnEvent != nil {
		return m.onOnEvent(ev)
	}
	return nil
}
