package tui

import (
	"strings"
	"testing"

	"crobot/internal/agent"
	"crobot/internal/config"
)

func testModel() Model {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil)
	m.pending = true
	m.agentEvents = make(chan agent.Event, 1)
	return *m
}

func TestHandleAgentEvent_TurnEndKeepsPending(t *testing.T) {
	m := testModel()

	updated, cmd := m.handleAgentEvent(agent.Event{Type: "turn_end"})
	m = updated.(Model)
	if !m.pending {
		t.Fatal("expected turn_end to keep pending true")
	}
	if cmd == nil {
		t.Fatal("expected turn_end to keep waiting for agent events")
	}
}

func TestHandleAgentEvent_MessageEndClearsPending(t *testing.T) {
	m := testModel()

	updated, cmd := m.handleAgentEvent(agent.Event{Type: "message_end"})
	m = updated.(Model)
	if m.pending {
		t.Fatal("expected message_end to clear pending")
	}
	if cmd != nil {
		t.Fatal("expected message_end to stop waiting for agent events")
	}
}

func TestNewModelConfiguresTextareaInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil)

	if m.textarea.Prompt != "" {
		t.Fatalf("expected empty textarea prompt, got %q", m.textarea.Prompt)
	}
	if m.textarea.ShowLineNumbers {
		t.Fatal("expected line numbers to be hidden")
	}
	if strings.Contains(m.renderInputView(), "Type a message") {
		t.Fatalf("expected no textarea placeholder, got %q", m.renderInputView())
	}
	if !strings.Contains(m.renderInputView(), "█") {
		t.Fatalf("expected block input cursor, got %q", m.renderInputView())
	}
}

func TestViewUsesInputViewForInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport.Width = 80
	m.viewport.Height = 20
	m.textarea.SetValue("hello")

	view := m.View()
	if !strings.Contains(view, "> hello") {
		t.Fatalf("expected input line to include textarea content, got %q", view)
	}
}
