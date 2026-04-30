package conversation

import (
	"testing"

	"crobot/internal/provider"
)

func TestMessagesToProvider_User(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hello"},
	}
	result := MessagesToProvider(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != RoleUser || result[0].Content != "hello" {
		t.Errorf("unexpected message: %+v", result[0])
	}
}

func TestMessagesToProvider_System(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "you are helpful"},
	}
	result := MessagesToProvider(msgs)
	if len(result) != 1 || result[0].Role != RoleSystem {
		t.Errorf("expected system role, got %+v", result[0])
	}
}

func TestMessagesToProvider_Compaction(t *testing.T) {
	msgs := []Message{
		{Role: RoleCompaction, Content: "summary text"},
	}
	result := MessagesToProvider(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != RoleSystem {
		t.Errorf("compaction role should become system, got %s", result[0].Role)
	}
}

func TestMessagesToProvider_AssistantPlain(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, Content: "hello", Reasoning: "thinking..."},
	}
	result := MessagesToProvider(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != RoleAssistant || result[0].Content != "hello" {
		t.Errorf("unexpected message: %+v", result[0])
	}
	if result[0].ReasoningContent != "thinking..." {
		t.Errorf("reasoning not preserved: %+v", result[0])
	}
}

func TestMessagesToProvider_AssistantWithTools(t *testing.T) {
	msgs := []Message{
		{
			Role:    RoleAssistant,
			Content: "let me check",
			ToolCalls: []ToolResult{
				{Name: "echo", CallID: "call_1", Args: map[string]any{"msg": "hi"}, Output: "hi"},
				{Name: "ls", CallID: "call_2", Args: map[string]any{"path": "."}, Output: "file.go\n"},
			},
		},
	}
	result := MessagesToProvider(msgs)
	// Assistant message + 2 tool messages = 3
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (assistant + 2 tool results), got %d", len(result))
	}
	if result[0].Role != RoleAssistant {
		t.Errorf("expected assistant, got %s", result[0].Role)
	}
	if len(result[0].ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls on assistant message, got %d", len(result[0].ToolCalls))
	}
	if result[1].Role != RoleTool || result[1].ToolCallID != "call_1" {
		t.Errorf("unexpected tool message: %+v", result[1])
	}
	if result[2].Role != RoleTool || result[2].ToolCallID != "call_2" {
		t.Errorf("unexpected tool message: %+v", result[2])
	}
}

func TestMessagesToProvider_EmptyToolCallSkipped(t *testing.T) {
	// Tool results with no output are not emitted as messages.
	msgs := []Message{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolResult{
				{Name: "bash", CallID: "call_1", Args: map[string]any{"cmd": "ls"}, Output: ""},
			},
		},
	}
	result := MessagesToProvider(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message (no tool output), got %d", len(result))
	}
}

func TestMessagesToProvider_Mixed(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolResult{
			{Name: "read", CallID: "c1", Args: map[string]any{"path": "f.go"}, Output: "content"},
		}},
		{Role: RoleUser, Content: "thanks"},
		{Role: RoleCompaction, Content: "summary"},
	}
	result := MessagesToProvider(msgs)
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(result))
	}
	roles := make([]string, len(result))
	for i, m := range result {
		roles[i] = m.Role
	}
	expected := []string{"user", "assistant", "tool", "user", "system"}
	for i, exp := range expected {
		if roles[i] != exp {
			t.Errorf("message %d: expected role %s, got %s", i, exp, roles[i])
		}
	}
}

func TestProviderToMessages_Roundtrip(t *testing.T) {
	original := []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "let me check", Reasoning: "hmm", ToolCalls: []ToolResult{
			{Name: "read", CallID: "c1", Args: map[string]any{"path": "f.go"}, ArgsStr: "f.go", Output: "package main"},
			{Name: "bash", CallID: "c2", Args: map[string]any{"cmd": "ls"}, ArgsStr: "ls", Output: "f.go"},
		}},
		{Role: RoleUser, Content: "ok"},
	}

	// Convert to provider messages and back.
	providerMsgs := MessagesToProvider(original)
	roundtripped := ProviderToMessages(providerMsgs)

	if len(roundtripped) != len(original) {
		t.Fatalf("roundtrip changed message count: %d -> %d", len(original), len(roundtripped))
	}

	// Check tool calls survived.
	if len(roundtripped[1].ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls after roundtrip, got %d", len(roundtripped[1].ToolCalls))
	}
	if roundtripped[1].ToolCalls[0].Output != "package main" {
		t.Errorf("tool output lost: %q", roundtripped[1].ToolCalls[0].Output)
	}
	if roundtripped[1].ToolCalls[1].Output != "f.go" {
		t.Errorf("tool output lost: %q", roundtripped[1].ToolCalls[1].Output)
	}
	if roundtripped[1].Reasoning != "hmm" {
		t.Errorf("reasoning lost: %q", roundtripped[1].Reasoning)
	}
}

func TestMessagesToProvider_PreservesUsage(t *testing.T) {
	usage := &provider.Usage{InputTokens: 100, OutputTokens: 50}
	msgs := []Message{
		{Role: RoleAssistant, Content: "done", Usage: usage},
	}
	// Usage is stored on the conversation.Message but not transferred to
	// provider.Message (the LLM doesn't need it). This test just confirms
	// the conversion doesn't panic or drop the message.
	result := MessagesToProvider(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "done" {
		t.Errorf("content lost: %+v", result[0])
	}
}
