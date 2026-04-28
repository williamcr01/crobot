package tui

import (
	"fmt"
	"strings"
	"testing"

	"crobot/internal/agent"
	"crobot/internal/config"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

func TestUpdateRoutesPageKeysToViewport(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil)
	m.ready = true
	m.width = 80
	m.height = 10
	m.viewport = viewportWithContent(80, 5, 30)
	m.viewport.GotoBottom()
	before := m.viewport.YOffset

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	updatedModel := updated.(Model)

	if updatedModel.viewport.YOffset >= before {
		t.Fatalf("expected page up to scroll viewport up, before=%d after=%d", before, updatedModel.viewport.YOffset)
	}
}

func TestRefreshViewportPreservesManualScrollPosition(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil)
	m.viewport = viewport.New(80, 5)
	for i := 0; i < 30; i++ {
		m.messages = append(m.messages, messageItem{role: "system", content: fmt.Sprintf("line %02d", i)})
	}
	m.refreshViewport()
	m.viewport.LineUp(2)
	before := m.viewport.YOffset
	m.messages = append(m.messages, messageItem{role: "system", content: "new message"})

	m.refreshViewport()

	if m.viewport.YOffset != before {
		t.Fatalf("expected refresh to preserve manual scroll position, before=%d after=%d", before, m.viewport.YOffset)
	}
}

func viewportWithContent(width, height, lines int) viewport.Model {
	vp := viewport.New(width, height)
	var b strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	vp.SetContent(b.String())
	return vp
}
