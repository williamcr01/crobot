package runtime

import (
	"context"
	"strings"
	"testing"

	"crobot/internal/agent"
	"crobot/internal/config"
	"crobot/internal/conversation"
	"crobot/internal/provider"
	"crobot/internal/tools"
)

type mockProvider struct {
	req provider.Request
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Send(context.Context, provider.Request) (*provider.Response, error) {
	return nil, nil
}

func (m *mockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	m.req = req
	ch := make(chan provider.StreamEvent, 2)
	ch <- provider.StreamEvent{TextDelta: "ok"}
	ch <- provider.StreamEvent{Done: &provider.Usage{InputTokens: 1, OutputTokens: 1}}
	close(ch)
	return ch, nil
}

func (m *mockProvider) ListModels(context.Context) ([]string, error) { return nil, nil }

func TestRunAgentUsesCanonicalConversationMessages(t *testing.T) {
	prov := &mockProvider{}
	var events []agent.Event

	result, err := RunAgent(context.Background(), AgentRequest{
		Config:   &config.AgentConfig{Model: "mock-model", Thinking: "none", MaxTurns: 1},
		Provider: prov,
		ToolReg:  tools.NewRegistry(),
		CWD:      "/tmp/project",
		Messages: []conversation.Message{
			{Role: conversation.RoleUser, Content: "hello"},
			{Role: conversation.RoleCompaction, Content: "summary"},
		},
		OnEvent: func(ev agent.Event) { events = append(events, ev) },
	})
	if err != nil {
		t.Fatalf("RunAgent returned error: %v", err)
	}
	if result.Text != "ok" {
		t.Fatalf("expected result text ok, got %q", result.Text)
	}
	if prov.req.Model != "mock-model" {
		t.Fatalf("expected model mock-model, got %q", prov.req.Model)
	}
	if !strings.Contains(prov.req.SystemPrompt, "/tmp/project") {
		t.Fatalf("expected system prompt to include cwd, got %q", prov.req.SystemPrompt)
	}
	if len(prov.req.Messages) != 2 {
		t.Fatalf("expected 2 provider messages, got %d", len(prov.req.Messages))
	}
	if prov.req.Messages[1].Role != conversation.RoleSystem {
		t.Fatalf("expected compaction message to become system, got %q", prov.req.Messages[1].Role)
	}
	if len(events) == 0 {
		t.Fatal("expected streamed agent events")
	}
}

func TestRunAgentValidatesRequiredDependencies(t *testing.T) {
	if _, err := RunAgent(context.Background(), AgentRequest{}); err == nil {
		t.Fatal("expected missing config error")
	}
	if _, err := RunAgent(context.Background(), AgentRequest{Config: &config.AgentConfig{}}); err == nil {
		t.Fatal("expected missing provider error")
	}
}
