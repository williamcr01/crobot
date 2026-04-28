package tui

import (
	"fmt"
	"os"
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

func TestNewModelShowsProviderWarningWhenAuthMissing(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil)

	if len(m.messages) != 1 || m.messages[0].role != "error" || !strings.Contains(m.messages[0].content, "No provider added") {
		t.Fatalf("expected no provider warning, got %#v", m.messages)
	}
}

func TestNewModelDoesNotWarnWhenAuthExistsButProviderUnselected(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil)

	if len(m.messages) != 0 {
		t.Fatalf("expected no startup warning, got %#v", m.messages)
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

func TestViewShowsProviderModelAndThinkingAboveInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "test/model", Thinking: "medium"}, nil, nil, nil, nil, nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport.Width = 80
	m.viewport.Height = 19

	view := m.View()
	status := "provider: openrouter  model: test/model  thinking: medium"
	input := "> "
	statusIndex := strings.Index(view, status)
	inputIndex := strings.LastIndex(view, input)
	if statusIndex == -1 {
		t.Fatalf("expected status line %q in view %q", status, view)
	}
	if inputIndex == -1 || statusIndex > inputIndex {
		t.Fatalf("expected status line above input, view %q", view)
	}
}

func TestTabCyclesThinkingLevels(t *testing.T) {
	withTempWorkingDir(t)
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Thinking: "none"}, nil, nil, nil, nil, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updatedModel := updated.(Model)

	if updatedModel.config.Thinking != "minimal" {
		t.Fatalf("expected thinking to cycle to minimal, got %q", updatedModel.config.Thinking)
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected agent.config.json to be written: %v", err)
	}
	if !strings.Contains(string(data), `"thinking": "minimal"`) {
		t.Fatalf("expected persisted thinking level, got %s", string(data))
	}
}

func TestTabDoesNotCycleThinkingWhenPending(t *testing.T) {
	m := NewModel(&config.AgentConfig{Thinking: "none"}, nil, nil, nil, nil, nil)
	m.pending = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updatedModel := updated.(Model)

	if updatedModel.config.Thinking != "none" {
		t.Fatalf("expected pending tab to leave thinking unchanged, got %q", updatedModel.config.Thinking)
	}
}

func TestRenderMessagesShowsReasoningWhenEnabled(t *testing.T) {
	m := NewModel(&config.AgentConfig{Display: config.DisplayConfig{Reasoning: true}}, nil, nil, nil, nil, nil)
	m.messages = append(m.messages, messageItem{role: "assistant", reasoning: "hidden chain", content: "final answer"})

	got := m.renderMessages()
	if !strings.Contains(got, "thinking") || !strings.Contains(got, "hidden chain") {
		t.Fatalf("expected reasoning to render, got %q", got)
	}
	if !strings.Contains(got, "final answer") {
		t.Fatalf("expected assistant content to render, got %q", got)
	}
}

func TestRenderMessagesHidesReasoningWhenDisabled(t *testing.T) {
	m := NewModel(&config.AgentConfig{Display: config.DisplayConfig{Reasoning: false}}, nil, nil, nil, nil, nil)
	m.messages = append(m.messages, messageItem{role: "assistant", reasoning: "hidden chain", content: "final answer"})

	got := m.renderMessages()
	if strings.Contains(got, "thinking") || strings.Contains(got, "hidden chain") {
		t.Fatalf("expected reasoning to be hidden, got %q", got)
	}
	if !strings.Contains(got, "final answer") {
		t.Fatalf("expected assistant content to render, got %q", got)
	}
}

func TestUpdateRoutesReasoningDeltaToReasoningField(t *testing.T) {
	m := NewModel(&config.AgentConfig{Display: config.DisplayConfig{Reasoning: true}}, nil, nil, nil, nil, nil)
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.Update(agentEventMsg(agent.Event{Type: "reasoning_delta", ReasoningDelta: "thinking..."}))
	updatedModel := updated.(Model)

	if updatedModel.messages[0].reasoning != "thinking..." {
		t.Fatalf("expected reasoning field to update, got %q", updatedModel.messages[0].reasoning)
	}
	if updatedModel.messages[0].content != "" {
		t.Fatalf("reasoning should not be appended to content, got %q", updatedModel.messages[0].content)
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

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWd); err != nil {
			t.Fatal(err)
		}
	})
}
