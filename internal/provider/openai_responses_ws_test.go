package provider

import "testing"

func TestOpenAIResponsesWSNextCreateBodyStartsWithFullInput(t *testing.T) {
	p := &OpenAIResponsesWSProvider{prevID: "", lastLen: 0}
	body, expectedLen, err := p.nextCreateBody(Request{
		Model:        "gpt-5.5",
		SystemPrompt: "sys",
		Messages:     []Message{{Role: "user", Content: "hi"}},
		Tools:        []ToolDefinition{{Name: "read", Description: "read file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["previous_response_id"] != nil {
		t.Fatalf("unexpected previous_response_id: %#v", body["previous_response_id"])
	}
	if expectedLen != 1 {
		t.Fatalf("expected len 1, got %d", expectedLen)
	}
	input, ok := body["input"].([]map[string]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected one input item, got %#v", body["input"])
	}
	if input[0]["type"] != "message" || input[0]["role"] != "user" {
		t.Fatalf("unexpected input item: %#v", input[0])
	}
	if tools, ok := body["tools"].([]map[string]any); !ok || len(tools) != 1 {
		t.Fatalf("expected one tool, got %#v", body["tools"])
	}
}

func TestOpenAIResponsesWSNextCreateBodyUsesIncrementalInput(t *testing.T) {
	p := &OpenAIResponsesWSProvider{prevID: "resp_123", lastLen: 1}
	body, expectedLen, err := p.nextCreateBody(Request{
		Model: "gpt-5.5",
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", ToolCalls: []ToolCall{{Name: "read", ID: "call_1", Args: map[string]any{"path": "README.md"}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "contents"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["previous_response_id"] != "resp_123" {
		t.Fatalf("expected previous response id, got %#v", body["previous_response_id"])
	}
	if expectedLen != 3 {
		t.Fatalf("expected len 3, got %d", expectedLen)
	}
	input, ok := body["input"].([]map[string]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected only tool output input, got %#v", body["input"])
	}
	if input[0]["type"] != "function_call_output" || input[0]["call_id"] != "call_1" {
		t.Fatalf("unexpected incremental input: %#v", input[0])
	}
}

func TestHandleResponsesWSEventCapturesResponseIDAndUsage(t *testing.T) {
	ch := make(chan StreamEvent, 1)
	stop, responseID := handleResponsesWSEvent([]byte(`{"type":"response.completed","response":{"id":"resp_abc","usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":4}}}}`), ch)
	if !stop {
		t.Fatal("expected stop")
	}
	if responseID != "resp_abc" {
		t.Fatalf("expected response id, got %q", responseID)
	}
	ev := <-ch
	if ev.Done == nil || ev.Done.InputTokens != 10 || ev.Done.OutputTokens != 2 || ev.Done.CacheReadTokens != 4 {
		t.Fatalf("unexpected usage event: %#v", ev)
	}
}
