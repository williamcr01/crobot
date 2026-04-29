package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"crobot/internal/agent"
	"crobot/internal/config"
	"crobot/internal/provider"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func testModel() Model {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)
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

func TestHandleAgentEvent_ErrorClearsPending(t *testing.T) {
	m := testModel()

	updated, cmd := m.handleAgentEvent(agent.Event{Type: "error", Error: fmt.Errorf("provider failed")})
	m = updated.(Model)
	if m.pending {
		t.Fatal("expected error to clear pending")
	}
	if cmd != nil {
		t.Fatal("expected error to stop waiting for agent events")
	}
	if len(m.messages) == 0 || m.messages[len(m.messages)-1].role != "error" {
		t.Fatalf("expected error message to be appended, got %#v", m.messages)
	}
}

func TestWaitForEvents_ReturnsAgentDoneWhenChannelClosed(t *testing.T) {
	m := testModel()
	close(m.agentEvents)

	cmd := m.waitForEvents()
	msg := cmd()

	if _, ok := msg.(agentDoneMsg); !ok {
		t.Fatalf("expected agentDoneMsg when channel is closed, got %T", msg)
	}
}

func TestAgentDoneMsg_ClearsPending(t *testing.T) {
	m := testModel()
	m.pending = true
	m.agentEvents = make(chan agent.Event, 1) // open channel

	updated, _ := m.Update(agentDoneMsg{})
	newModel := updated.(Model)

	if newModel.pending {
		t.Fatal("expected agentDoneMsg to clear pending")
	}
}

func TestNewModelShowsProviderWarningWhenAuthMissing(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)

	if len(m.messages) != 1 || m.messages[0].role != "error" || !strings.Contains(m.messages[0].content, "No provider added") {
		t.Fatalf("expected no provider warning, got %#v", m.messages)
	}
}

func TestNewModelDoesNotWarnWhenAuthExistsButProviderUnselected(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil)

	if len(m.messages) != 0 {
		t.Fatalf("expected no startup warning, got %#v", m.messages)
	}
}

func TestNewModelConfiguresTextareaInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)

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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)
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
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "test/model", Thinking: "medium"}, nil, nil, nil, nil, nil, nil, nil)
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
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Thinking: "none"}, nil, nil, nil, nil, nil, nil, nil)

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
	m := NewModel(&config.AgentConfig{Thinking: "none"}, nil, nil, nil, nil, nil, nil, nil)
	m.pending = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updatedModel := updated.(Model)

	if updatedModel.config.Thinking != "none" {
		t.Fatalf("expected pending tab to leave thinking unchanged, got %q", updatedModel.config.Thinking)
	}
}

func TestSelectModelRecreatesProviderWhenProviderChanges(t *testing.T) {
	deepseek, err := provider.Create("deepseek", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(&config.AgentConfig{Provider: "deepseek", Model: "deepseek-v4-pro"}, deepseek, nil, nil, nil, nil, nil, func(name string) string {
		return map[string]string{"openrouter": "sk-or-test", "deepseek": "sk-test"}[name]
	})

	m.selectModel("openrouter", "openai/gpt-4o")

	if m.provider == nil {
		t.Fatal("expected provider to be recreated")
	}
	if got := m.provider.Name(); got != "openrouter" {
		t.Fatalf("expected openrouter provider after switching model, got %q", got)
	}
}

func TestSelectModelClearsStaleProviderWhenNewProviderUnauthorized(t *testing.T) {
	deepseek, err := provider.Create("deepseek", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(&config.AgentConfig{Provider: "deepseek", Model: "deepseek-v4-pro"}, deepseek, nil, nil, nil, nil, nil, func(name string) string {
		if name == "deepseek" {
			return "sk-test"
		}
		return ""
	})

	m.selectModel("openrouter", "openai/gpt-4o")

	if m.provider != nil {
		t.Fatalf("expected stale provider to be cleared, got %q", m.provider.Name())
	}
}

func TestRenderMessagesShowsReasoningWhenEnabled(t *testing.T) {
	m := NewModel(&config.AgentConfig{Reasoning: true}, nil, nil, nil, nil, nil, nil, nil)
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
	m := NewModel(&config.AgentConfig{Reasoning: false}, nil, nil, nil, nil, nil, nil, nil)
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
	m := NewModel(&config.AgentConfig{Reasoning: true}, nil, nil, nil, nil, nil, nil, nil)
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)
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

func TestOAuthProviderOptionUsesOpenAIOAuthID(t *testing.T) {
	providers := oauthProviderOptions()
	if len(providers) != 1 {
		t.Fatalf("expected one oauth provider, got %d", len(providers))
	}
	if providers[0].ID != "openai-codex" {
		t.Fatalf("expected openai-codex provider ID, got %q", providers[0].ID)
	}
}

func TestEscCancelsPendingAgent(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	m.pending = true
	m.agentCancel = cancel
	m.agentEvents = make(chan agent.Event, 1)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updatedModel := updated.(Model)

	if updatedModel.pending {
		t.Fatal("expected ESC to clear pending")
	}

	// Verify context was cancelled.
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected context to be cancelled after ESC")
	}

	// Verify a system message was appended.
	if len(updatedModel.messages) == 0 || updatedModel.messages[len(updatedModel.messages)-1].content != "Cancelled." {
		t.Fatalf("expected Cancelled. system message, got %#v", updatedModel.messages)
	}
}

func TestWrapLine_ShortLine(t *testing.T) {
	got := wrapLine("hello", 10)
	if got != "hello" {
		t.Fatalf("expected no wrapping for short line, got %q", got)
	}
}

func TestWrapLine_WrapsAtWordBoundary(t *testing.T) {
	got := wrapLine("hello world foo", 12)
	expected := "hello world\nfoo"
	if got != expected {
		t.Fatalf("expected wrapped at space, got %q", got)
	}
}

func TestWrapLine_ForceBreaksLongWord(t *testing.T) {
	got := wrapLine("abcdefghijklmnopqrstuvwxyz", 10)
	// Force-break at position 10: "abcdefghij" then "klmnopqrst" then "uvwxyz"
	expected := "abcdefghij\nklmnopqrst\nuvwxyz"
	if got != expected {
		t.Fatalf("expected force-break, got %q", got)
	}
}

func TestWrapLine_EmptyString(t *testing.T) {
	got := wrapLine("", 10)
	if got != "" {
		t.Fatalf("expected empty for empty input, got %q", got)
	}
}

func TestWrapText_PreservesNewlines(t *testing.T) {
	got := wrapText("line one\nline two is longer than width", 15)
	expected := "line one\nline two is\nlonger than\nwidth"
	if got != expected {
		t.Fatalf("expected preserved newlines with wrapping, got %q", got)
	}
}

func TestWrapText_WrapsAllMessages(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil)
	m.width = 40
	m.messages = append(m.messages, messageItem{role: "assistant", content: "this is a long response that should wrap to multiple lines"})
	m.messages = append(m.messages, messageItem{role: "system", content: "this is a system message that is also very long and must wrap"})

	got := m.renderMessages()
	// Strip ANSI escape sequences for length checking.
	clean := stripANSI(got)
	for _, line := range strings.Split(clean, "\n") {
		if len(line) > m.width {
			t.Fatalf("visible line exceeds viewport width %d (len=%d): %q", m.width, len(line), line)
		}
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if s[i] >= '@' && s[i] <= '~' {
				inEscape = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestEscDoesNothingWhenNotPending(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)

	msgCount := len(m.messages)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updatedModel := updated.(Model)

	if updatedModel.pending {
		t.Fatal("expected ESC to have no effect when not pending")
	}
	if len(updatedModel.messages) != msgCount {
		t.Fatalf("expected no new messages when not pending, had %d got %d", msgCount, len(updatedModel.messages))
	}
}
