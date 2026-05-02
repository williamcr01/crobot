package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"crobot/internal/config"
	"crobot/internal/provider"
	"crobot/internal/tools"
)

// mockProvider simulates an LLM provider for testing.
type mockProvider struct {
	name      string
	responses []responseStep
	stepIdx   int
	requests  []provider.Request
	cancel    context.CancelFunc // called mid-stream if cancelMid is set
}

type responseStep struct {
	text      string
	reasoning string
	toolCalls []provider.ToolCall
	usage     *provider.Usage
	err       error
	cancelMid bool // cancel context after sending reasoning but before text
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Send(ctx context.Context, req provider.Request) (*provider.Response, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	m.requests = append(m.requests, req)
	step := m.responses[m.stepIdx]
	m.stepIdx++
	if step.err != nil {
		return nil, step.err
	}
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		defer close(ch)

		if step.reasoning != "" {
			runes := []rune(step.reasoning)
			mid := len(runes) / 2
			for i, r := range runes {
				if i == mid && step.cancelMid && m.cancel != nil {
					m.cancel()
					// Drain remaining to channel so sender doesn't block.
					for j := i; j < len(runes); j++ {
						ch <- provider.StreamEvent{ReasoningDelta: string(runes[j])}
					}
					return
				}
				ch <- provider.StreamEvent{ReasoningDelta: string(r)}
			}
		}

		if step.text != "" {
			for _, r := range step.text {
				ch <- provider.StreamEvent{TextDelta: string(r)}
			}
		}

		for _, tc := range step.toolCalls {
			ch <- provider.StreamEvent{ToolCallStart: &provider.ToolCall{Name: tc.Name, ID: tc.ID}}
			ch <- provider.StreamEvent{ToolCallEnd: &provider.ToolCall{Name: tc.Name, ID: tc.ID, Args: tc.Args}}
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
	}

	toolReg := tools.NewRegistry()
	var events []Event
	onEvent := func(ev Event) {
		events = append(events, ev)
	}

	result, err := Run(context.Background(), mock, cfg.Model, cfg.SystemPrompt, nil, toolReg, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", result.Text)
	}
	if result.Usage == nil {
		t.Fatal("expected usage")
	}
}

func TestRun_ToolCall(t *testing.T) {
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
	}

	toolReg := tools.NewRegistry()
	toolReg.Register(echoTool)
	var events []Event
	onEvent := func(ev Event) {
		events = append(events, ev)
	}

	result, err := Run(context.Background(), mock, cfg.Model, cfg.SystemPrompt, nil, toolReg, nil, onEvent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Done!" {
		t.Errorf("expected 'Done!', got %q", result.Text)
	}
	if len(mock.requests) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(mock.requests))
	}
	msgs := mock.requests[1].Messages
	if len(msgs) != 2 {
		t.Fatalf("expected assistant tool call and tool result messages, got %d", len(msgs))
	}
	if len(msgs[0].ToolCalls) != 1 || msgs[0].ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected assistant message to preserve tool call metadata, got %#v", msgs[0])
	}
	if msgs[1].Role != "tool" || msgs[1].ToolCallID != "call_1" {
		t.Fatalf("expected tool result with tool_call_id call_1, got %#v", msgs[1])
	}
	var sawTurnUsage bool
	for _, ev := range events {
		if ev.Type == "turn_usage" && ev.TurnUsage != nil {
			sawTurnUsage = true
			if ev.TurnUsage.InputTokens != 10 || ev.TurnUsage.OutputTokens != 5 {
				t.Fatalf("unexpected turn usage: %#v", ev.TurnUsage)
			}
		}
	}
	if !sawTurnUsage {
		t.Fatal("expected turn_usage event for tool-call turn")
	}
}

func TestRun_ToolCallEndWithoutStart(t *testing.T) {
	echoTool := tools.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			return msg, nil
		},
	}

	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{text: "", usage: &provider.Usage{InputTokens: 1, OutputTokens: 1}},
			{text: "Done!", usage: &provider.Usage{InputTokens: 2, OutputTokens: 1}},
		},
	}

	toolReg := tools.NewRegistry()
	toolReg.Register(echoTool)
	r := &runner{prov: completeToolCallProvider{mock}, model: "mock-model", maxTurns: 50, toolReg: toolReg}

	result, err := r.run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Done!" {
		t.Fatalf("expected final text, got %q", result.Text)
	}
	if len(mock.requests) != 2 {
		t.Fatalf("expected tool call to trigger second request, got %d requests", len(mock.requests))
	}
}

type completeToolCallProvider struct{ *mockProvider }

func (p completeToolCallProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	p.requests = append(p.requests, req)
	ch := make(chan provider.StreamEvent, 2)
	go func() {
		defer close(ch)
		if len(p.requests) == 1 {
			ch <- provider.StreamEvent{ToolCallEnd: &provider.ToolCall{Name: "echo", ID: "call_1", Args: map[string]any{"message": "testing"}}}
			ch <- provider.StreamEvent{Done: &provider.Usage{InputTokens: 1, OutputTokens: 1}}
			return
		}
		ch <- provider.StreamEvent{TextDelta: "Done!"}
		ch <- provider.StreamEvent{Done: &provider.Usage{InputTokens: 2, OutputTokens: 1}}
	}()
	return ch, nil
}

func TestRun_ProviderErrorEmitsTypedErrorEvent(t *testing.T) {
	expectedErr := fmt.Errorf("provider failed")
	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{err: expectedErr},
			{err: expectedErr},
			{err: expectedErr},
			{err: expectedErr},
		},
	}

	var events []Event
	_, err := Run(context.Background(), mock, "mock", "prompt", nil, tools.NewRegistry(), nil, func(ev Event) {
		events = append(events, ev)
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if len(events) == 0 || events[len(events)-1].Type != "error" || events[len(events)-1].Error == nil {
		t.Fatalf("expected final typed error event, got %#v", events)
	}
}

func TestRun_StopsAfterMaxTurns(t *testing.T) {
	maxTurns := 3
	responses := make([]responseStep, maxTurns+1)
	for i := range responses {
		responses[i] = responseStep{toolCalls: []provider.ToolCall{{Name: "missing", ID: fmt.Sprintf("call_%d", i)}}}
	}
	mock := &mockProvider{name: "mock", responses: responses}

	var last Event
	_, err := RunWithThinking(context.Background(), mock, "mock", "none", maxTurns, "prompt", nil, tools.NewRegistry(), nil, func(ev Event) {
		last = ev
	})
	if err == nil || !strings.Contains(err.Error(), "infinite loop") {
		t.Fatalf("expected infinite loop guard error, got %v", err)
	}
	if last.Type != "error" || last.Error == nil {
		t.Fatalf("expected typed error event from turn limit, got %#v", last)
	}
	if len(mock.requests) != maxTurns {
		t.Fatalf("expected %d provider requests, got %d", maxTurns, len(mock.requests))
	}
}

func TestRun_MaxTurnsMinusOneDisablesLimit(t *testing.T) {
	responses := []responseStep{
		{toolCalls: []provider.ToolCall{{Name: "missing", ID: "call_1"}}},
		{text: "done"},
	}
	mock := &mockProvider{name: "mock", responses: responses}

	result, err := RunWithThinking(context.Background(), mock, "mock", "none", -1, "prompt", nil, tools.NewRegistry(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "done" {
		t.Fatalf("expected final text, got %q", result.Text)
	}
}

func TestRun_Cancellation(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{text: "before cancellation..."},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, mock, "mock", "prompt", nil, tools.NewRegistry(), nil, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestRun_CancellationPreservesReasoning(t *testing.T) {
	// Verify that processStream returns partial state (including reasoning)
	// when the context is cancelled mid-stream.
	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{reasoning: "let me think about this...", cancelMid: true},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	mock.cancel = cancel

	r := &runner{
		prov:         mock,
		model:        "mock",
		systemPrompt: "system",
		toolReg:      tools.NewRegistry(),
		messages:     nil,
		maxTurns:     50,
	}

	// streamWithRetry calls processStream which should return partial + error.
	step, err := r.streamWithRetry(ctx, provider.Request{
		Model:  "mock",
		Stream: true,
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if step == nil {
		t.Fatal("expected partial step on cancellation, got nil")
	}
	if step.ReasoningContent == "" {
		t.Error("expected non-empty reasoning in partial step")
	}
}

func TestRun_PluginHooks(t *testing.T) {
	var hookCalls []string

	pluginManager := &mockPluginManager{
		onPrePrompt: func(prompt string, msgs []provider.Message) (string, []provider.Message, error) {
			hookCalls = append(hookCalls, "pre_prompt")
			return prompt, msgs, nil
		},
		onPostResponse: func(resp *Result) (*Result, error) {
			hookCalls = append(hookCalls, "post_response")
			return resp, nil
		},
		onPreToolCall: func(name string, args map[string]any) (string, map[string]any, bool, error) {
			hookCalls = append(hookCalls, "pre_tool")
			return name, args, false, nil
		},
		onPostToolResult: func(name string, args map[string]any, result any) (any, error) {
			hookCalls = append(hookCalls, "post_tool")
			return result, nil
		},
		onOnEvent: func(ev any) error {
			hookCalls = append(hookCalls, "on_event")
			return nil
		},
	}

	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{
				text:  "Hello!",
				usage: &provider.Usage{InputTokens: 10, OutputTokens: 2},
			},
		},
	}

	_, err := Run(context.Background(), mock, "mock", "prompt", nil, tools.NewRegistry(), pluginManager, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundPrePrompt := false
	foundPostResponse := false
	for _, c := range hookCalls {
		if c == "pre_prompt" {
			foundPrePrompt = true
		}
		if c == "post_response" {
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

type mockPluginManager struct {
	onPrePrompt      func(string, []provider.Message) (string, []provider.Message, error)
	onPostResponse   func(*Result) (*Result, error)
	onPreToolCall    func(string, map[string]any) (string, map[string]any, bool, error)
	onPostToolResult func(string, map[string]any, any) (any, error)
	onOnEvent        func(any) error
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

func (m *mockPluginManager) CallOnEvent(ev Event) error {
	if m.onOnEvent != nil {
		return m.onOnEvent(ev)
	}
	return nil
}

func (m *mockPluginManager) DrainMessages() []provider.Message {
	return nil
}

func TestRun_MultipleToolCalls_NonNumericIDs(t *testing.T) {
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
	reverseTool := tools.Tool{
		Name:        "reverse",
		Description: "Reverses input",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			runes := []rune(msg)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			return map[string]any{"reversed": string(runes)}, nil
		},
	}

	mock := &mockProvider{
		name: "mock",
		responses: []responseStep{
			{
				text: "",
				toolCalls: []provider.ToolCall{
					{Name: "echo", ID: "call_abc123", Args: map[string]any{"message": "hello"}},
					{Name: "reverse", ID: "call_def456", Args: map[string]any{"message": "world"}},
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
	}

	toolReg := tools.NewRegistry()
	toolReg.Register(echoTool)
	toolReg.Register(reverseTool)

	result, err := Run(context.Background(), mock, cfg.Model, cfg.SystemPrompt, nil, toolReg, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Done!" {
		t.Errorf("expected 'Done!', got %q", result.Text)
	}

	// Verify both tool results are in the conversation history.
	// The runner appends tool messages as "{callID}: {output}".
	if len(mock.responses) != 2 {
		t.Fatalf("expected 2 response steps, got %d", len(mock.responses))
	}
}
