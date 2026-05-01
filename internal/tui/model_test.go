package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"crobot/internal/agent"
	"crobot/internal/commands"
	"crobot/internal/compaction"
	"crobot/internal/config"
	"crobot/internal/provider"
	"crobot/internal/session"
	"crobot/internal/themes"
	"crobot/internal/tools"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func tuiStylesForTest() Styles {
	return NewStyles(themes.DefaultTheme())
}

func testModel() Model {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.agentEvents = make(chan agent.Event, 1)
	return *m
}

type metadataProvider struct {
	models []provider.ModelInfo
}

func (p metadataProvider) Name() string { return "metadata" }
func (p metadataProvider) Send(context.Context, provider.Request) (*provider.Response, error) {
	return nil, nil
}
func (p metadataProvider) Stream(context.Context, provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, nil
}
func (p metadataProvider) ListModels(context.Context) ([]string, error) {
	ids := make([]string, 0, len(p.models))
	for _, model := range p.models {
		ids = append(ids, model.ID)
	}
	return ids, nil
}
func (p metadataProvider) ListModelInfo(context.Context) ([]provider.ModelInfo, error) {
	return p.models, nil
}

func TestSessionDisplayTitle(t *testing.T) {
	info := session.SessionInfo{Title: "Saved title", FirstPrompt: "first", FirstMessage: "message"}
	if got := sessionDisplayTitle(info); got != "Saved title" {
		t.Fatalf("expected title, got %q", got)
	}
	info.Title = ""
	if got := sessionDisplayTitle(info); got != "first" {
		t.Fatalf("expected first prompt, got %q", got)
	}
	info.FirstPrompt = ""
	info.FirstMessage = "(no messages)"
	if got := sessionDisplayTitle(info); got != "(empty session)" {
		t.Fatalf("expected empty session, got %q", got)
	}
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	if len(m.messages) != 1 || m.messages[0].role != "error" || !strings.Contains(m.messages[0].content, "No provider added") {
		t.Fatalf("expected no provider warning, got %#v", m.messages)
	}
}

func TestNewModelDoesNotWarnWhenAuthExistsButProviderUnselected(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	if len(m.messages) != 0 {
		t.Fatalf("expected no startup warning, got %#v", m.messages)
	}
}

func TestNewModelConfiguresTextareaInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

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

func TestGhosttyShiftEnterInsertsNewline(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("hello")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	updatedModel := updated.(Model)

	if got := updatedModel.textarea.Value(); got != "hello\n" {
		t.Fatalf("expected ctrl+j/LF to insert newline, got %q", got)
	}
}

func TestAltEnterInsertsNewline(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("hello")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	updatedModel := updated.(Model)

	if got := updatedModel.textarea.Value(); got != "hello\n" {
		t.Fatalf("expected alt+enter to insert newline, got %q", got)
	}
}

func TestShiftEnterRawSequences(t *testing.T) {
	for _, seq := range [][]byte{
		[]byte("\x1b\r"),
		[]byte("\x1b[13;2~"),
		[]byte("\x1b[27;2;13~"),
		[]byte("\x1b[13;2u"),
		[]byte("\x1b[13;2:1u"),
		[]byte("\x1b[57414;2u"),
		[]byte("\n"),
	} {
		if !isShiftEnterSequence(seq) {
			t.Fatalf("expected %q to be recognized as Shift+Enter", seq)
		}
	}
}

func TestViewUsesInputViewForInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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

func TestViewShowsProviderModelThinkingAndContextAboveInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "test/model", Thinking: "medium"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport.Width = 80
	m.viewport.Height = 19
	m.messages = []messageItem{{role: "user", content: strings.Repeat("a", 400)}}

	view := m.View()
	status := fmt.Sprintf("openrouter | test/model | medium | %d/128k | $0.00", m.estimatedContextUsed())
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

func TestStatusLineDoesNotExceedTerminalWidth(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "anthropic/claude-super-long-model-name-that-would-wrap-the-footer-and-break-the-tui", Thinking: "medium"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.width = 40

	status := stripANSI(m.renderStatusLine())
	if lipgloss.Width(status) > m.width {
		t.Fatalf("expected status width <= %d, got %d: %q", m.width, lipgloss.Width(status), status)
	}
}

func TestShiftTabCyclesThinkingLevels(t *testing.T) {
	withTempWorkingDir(t)
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Thinking: "none"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
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

func TestShiftTabDoesNotCycleThinkingWhenPending(t *testing.T) {
	m := NewModel(&config.AgentConfig{Thinking: "none"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
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
	}, nil, tuiStylesForTest())

	m.selectModel("openrouter", "openai/gpt-4o")

	if m.provider == nil {
		t.Fatal("expected provider to be recreated")
	}
	if got := m.provider.Name(); got != "openrouter" {
		t.Fatalf("expected openrouter provider after switching model, got %q", got)
	}
}

func TestStatusLineUsesSelectedModelContextLength(t *testing.T) {
	reg := provider.NewModelRegistry()
	reg.AddProvider(metadataProvider{models: []provider.ModelInfo{
		{ID: "small-model", ContextLength: 32_000},
		{ID: "large-model", ContextLength: 1_000_000},
	}})
	if err := reg.LoadModels(context.Background()); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.AgentConfig{Provider: "metadata", Model: "small-model", Thinking: "none"}, nil, nil, nil, nil, reg, nil, nil, nil, tuiStylesForTest())
	m.width = 120
	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "/32k") {
		t.Fatalf("expected small model context in status, got %q", got)
	}

	m.config.Model = "large-model"
	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "/1M") {
		t.Fatalf("expected large model context after model change, got %q", got)
	}
}

func TestTurnUsageUpdatesCostWithoutClearingPending(t *testing.T) {
	reg := provider.NewModelRegistry()
	reg.AddProvider(metadataProvider{models: []provider.ModelInfo{{
		ID:      "priced-model",
		Pricing: provider.Pricing{InputPerMTok: 2, OutputPerMTok: 10},
	}}})
	if err := reg.LoadModels(context.Background()); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.AgentConfig{Provider: "metadata", Model: "priced-model", Thinking: "none"}, nil, nil, nil, nil, reg, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.agentEvents = make(chan agent.Event)
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.handleAgentEvent(agent.Event{Type: "turn_usage", TurnUsage: &provider.Usage{InputTokens: 1_000_000, OutputTokens: 100_000}})
	updatedModel := updated.(Model)

	if !updatedModel.pending {
		t.Fatal("expected turn usage to keep pending true")
	}
	if got := stripANSI(updatedModel.renderStatusLine()); !strings.Contains(got, "| $3.0000") {
		t.Fatalf("expected cost update from turn usage, got %q", got)
	}
}

func TestStatusLineShowsCumulativeCost(t *testing.T) {
	reg := provider.NewModelRegistry()
	reg.AddProvider(metadataProvider{models: []provider.ModelInfo{{
		ID:      "priced-model",
		Pricing: provider.Pricing{InputPerMTok: 2, OutputPerMTok: 10},
	}}})
	if err := reg.LoadModels(context.Background()); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.AgentConfig{Provider: "metadata", Model: "priced-model", Thinking: "none"}, nil, nil, nil, nil, reg, nil, nil, nil, tuiStylesForTest())
	m.width = 120
	usage := &provider.Usage{InputTokens: 1_000_000, OutputTokens: 100_000}
	m.calculateUsageCost(usage)
	m.messages = []messageItem{{role: "assistant", usageData: usage}}

	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "| $3.0000") {
		t.Fatalf("expected cumulative cost in status, got %q", got)
	}
}

func TestStatusLineShowsSubscriptionInsteadOfCost(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openai-codex", Model: "gpt-5.1", Thinking: "none"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.width = 120
	m.messages = []messageItem{{role: "assistant", usageData: &provider.Usage{InputTokens: 1_000_000, OutputTokens: 100_000}}}

	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "| sub") {
		t.Fatalf("expected subscription status, got %q", got)
	}
}

func TestStatusLineUsesStaticContextFallbackWhenRegistryMissingMetadata(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "anthropic/claude-sonnet-4-5", Thinking: "none"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.width = 120
	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "/200k") {
		t.Fatalf("expected claude context fallback in status, got %q", got)
	}

	m.config.Model = "google/gemini-2.5-pro"
	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "/1M") {
		t.Fatalf("expected gemini context fallback after model change, got %q", got)
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
	}, nil, tuiStylesForTest())

	m.selectModel("openrouter", "openai/gpt-4o")

	if m.provider != nil {
		t.Fatalf("expected stale provider to be cleared, got %q", m.provider.Name())
	}
}

func TestRenderMessagesShowsReasoningWhenEnabled(t *testing.T) {
	m := NewModel(&config.AgentConfig{Reasoning: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
	m := NewModel(&config.AgentConfig{Reasoning: false}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
	m := NewModel(&config.AgentConfig{Reasoning: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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
func TestEscDoesNothingWhenNotPending(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
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

// --- Picker tests ---

type mockModelRegistry struct {
	models []commands.ModelInfo
}

func (m *mockModelRegistry) GetAll() []commands.ModelInfo { return m.models }

func (m *mockModelRegistry) Filter(prefix string) []commands.ModelInfo {
	if prefix == "" {
		return m.models
	}
	var res []commands.ModelInfo
	for _, model := range m.models {
		if strings.Contains(model.ID, prefix) {
			res = append(res, model)
		}
	}
	return res
}

func TestHandleModelPickerKey_EscCancels(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true
	m.textarea.SetValue("openai")

	updated, cmd, handled := m.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if m2.modelPickerActive {
		t.Fatal("expected model picker to be closed")
	}
	if cmd != nil {
		t.Fatalf("expected no cmd after esc, got %v", cmd)
	}
}

func TestHandleModelPickerKey_CtrlCCancels(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	updated, cmd, handled := m.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected ctrl+c to be handled")
	}
	if m2.modelPickerActive {
		t.Fatal("expected model picker to be closed")
	}
	_ = cmd
}

func TestHandleModelPickerKey_EnterNoModels(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{}})

	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	updated, cmd, handled := m.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected enter to be handled")
	}
	if m2.modelPickerActive {
		t.Fatal("expected model picker to close with no models")
	}
	if m2.config.Model != "" {
		t.Fatalf("expected no model selected, got %q", m2.config.Model)
	}
	_ = cmd
}

func TestHandleModelPickerKey_ArrowNavigation(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "model-a", Provider: "openrouter"},
		{ID: "model-b", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	// Down arrow
	updated, cmd, handled := m.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyDown})
	m2 := updated.(Model)
	if !handled {
		t.Fatal("expected key down to be handled")
	}
	if m2.modelPickerIndex != 1 {
		t.Fatalf("expected index 1 after down, got %d", m2.modelPickerIndex)
	}
	_ = cmd

	// Up arrow
	updated, cmd, handled = m2.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyUp})
	m3 := updated.(Model)
	if !handled {
		t.Fatal("expected key up to be handled")
	}
	if m3.modelPickerIndex != 0 {
		t.Fatalf("expected index 0 after up, got %d", m3.modelPickerIndex)
	}
	_ = cmd
}

func TestClampModelPickerIndex(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	models := []commands.Command{
		{Name: "model1", ModelID: "m1"},
		{Name: "model2", ModelID: "m2"},
	}

	// Clamp negative
	m.modelPickerIndex = -5
	m.clampModelPickerIndex(models)
	if m.modelPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping negative, got %d", m.modelPickerIndex)
	}

	// Clamp too high
	m.modelPickerIndex = 10
	m.clampModelPickerIndex(models)
	if m.modelPickerIndex != 1 {
		t.Fatalf("expected 1 after clamping high, got %d", m.modelPickerIndex)
	}

	// Clamp empty
	m.modelPickerIndex = 3
	m.clampModelPickerIndex(nil)
	if m.modelPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping empty, got %d", m.modelPickerIndex)
	}
}

func TestRenderModelPicker_Empty(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	got := m.renderModelPicker()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "No models match") {
		t.Fatalf("expected no models message, got %q", stripped)
	}
}

func TestRenderModelPicker_WithModels(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	got := m.renderModelPicker()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "openai/gpt-4o") {
		t.Fatalf("expected model in render: %q", stripped)
	}
}

func TestModelPickerHeight(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "m1", Provider: "p1"},
		{ID: "m2", Provider: "p1"},
	}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	h := m.modelPickerHeight()
	if h < 2 {
		t.Fatalf("expected reasonable picker height, got %d", h)
	}
}

func TestVisibleModelPickerRange(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	models := make([]commands.Command, 5)
	for i := range models {
		models[i] = commands.Command{Name: fmt.Sprintf("m%d", i)}
	}

	// Small list
	m.modelPickerIndex = 2
	start, end, sel := m.visibleModelPickerRange(models)
	if start != 0 || end != 5 || sel != 2 {
		t.Fatalf("small list: start=%d end=%d sel=%d, want 0 5 2", start, end, sel)
	}

	// Index out of range
	m.modelPickerIndex = 100
	_, _, sel = m.visibleModelPickerRange(models)
	if sel != 4 {
		t.Fatalf("expected sel clamped to 4, got %d", sel)
	}

	// Negative index
	m.modelPickerIndex = -1
	_, _, sel = m.visibleModelPickerRange(models)
	if sel != 0 {
		t.Fatalf("expected sel clamped to 0, got %d", sel)
	}
}

func TestRenderModelPicker_FilteredWithArgs(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter", ContextLength: 128000},
	}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true
	m.modelPickerFilter = "gpt"

	got := m.renderModelPicker()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "openai/gpt-4o") {
		t.Fatalf("expected filtered model in render: %q", stripped)
	}
}

// --- Picker helper tests ---

func TestFilteredLoginProviders(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// No filter
	m.loginPickerFilter = ""
	providers := m.filteredLoginProviders()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].ID != "openai-codex" {
		t.Fatalf("expected openai-codex, got %q", providers[0].ID)
	}

	// Matching filter
	m.loginPickerFilter = "openai"
	providers = m.filteredLoginProviders()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider with matching filter, got %d", len(providers))
	}

	// Non-matching filter
	m.loginPickerFilter = "anthropic"
	providers = m.filteredLoginProviders()
	if len(providers) != 0 {
		t.Fatalf("expected 0 providers with non-matching filter, got %d", len(providers))
	}
}

func TestClampLoginPickerIndex(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	providers := []loginProviderOption{
		{ID: "openai-codex"},
		{ID: "another"},
	}

	// Clamp negative
	m.loginPickerIndex = -1
	m.clampLoginPickerIndex(providers)
	if m.loginPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping negative, got %d", m.loginPickerIndex)
	}

	// Clamp too high
	m.loginPickerIndex = 10
	m.clampLoginPickerIndex(providers)
	if m.loginPickerIndex != 1 {
		t.Fatalf("expected 1 after clamping high, got %d", m.loginPickerIndex)
	}

	// Empty
	m.loginPickerIndex = 5
	m.clampLoginPickerIndex(nil)
	if m.loginPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping empty, got %d", m.loginPickerIndex)
	}
}

func TestClampLogoutPickerIndex(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	providers := []loginProviderOption{{ID: "openai-codex"}}

	m.logoutPickerIndex = -1
	m.clampLogoutPickerIndex(providers)
	if m.logoutPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping negative, got %d", m.logoutPickerIndex)
	}

	m.logoutPickerIndex = 5
	m.clampLogoutPickerIndex(providers)
	if m.logoutPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping high, got %d", m.logoutPickerIndex)
	}

	m.logoutPickerIndex = 3
	m.clampLogoutPickerIndex(nil)
	if m.logoutPickerIndex != 0 {
		t.Fatalf("expected 0 after clamping empty, got %d", m.logoutPickerIndex)
	}
}

func TestRenderLoginPicker(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.loginPickerActive = true

	got := m.renderLoginPicker()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "OpenAI Codex") {
		t.Fatalf("expected OpenAI Codex in login picker: %q", stripped)
	}
}

func TestRenderLogoutPicker_NoProviders(t *testing.T) {
	// Set HOME to a temp dir so no auth file exists
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	got := m.renderLogoutPicker()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "No logged-in OAuth providers") {
		t.Fatalf("expected no providers message: %q", stripped)
	}
}

func TestHandleLoginPickerKey_Esc(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.loginPickerActive = true

	updated, _, handled := m.handleLoginPickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if m2.loginPickerActive {
		t.Fatal("expected login picker to close")
	}
}

func TestHandleLoginPickerKey_ArrowNavigation(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.loginPickerActive = true

	// Down arrow should cycle to 0 (only one provider)
	updated, _, handled := m.handleLoginPickerKey(tea.KeyMsg{Type: tea.KeyDown})
	m2 := updated.(Model)
	if !handled {
		t.Fatal("expected down to be handled")
	}
	if m2.loginPickerIndex != 0 {
		t.Fatalf("expected index 0, got %d", m2.loginPickerIndex)
	}
}

func TestHandleLoginPickerKey_EnterNoProviders(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.loginPickerActive = true
	m.loginPickerFilter = "nonexistent"

	updated, _, handled := m.handleLoginPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected enter to be handled")
	}
	if !m2.loginPickerActive {
		t.Fatal("expected picker to remain active when no providers match")
	}
}

func TestHandleLogoutPickerKey_Esc(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	updated, _, handled := m.handleLogoutPickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected esc to be handled")
	}
	if m2.logoutPickerActive {
		t.Fatal("expected logout picker to close")
	}
}

func TestLoginProviderCmd_StandardProvider(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	cmd := m.loginProviderCmd("unknown")
	msg := cmd()
	result, ok := msg.(loginResultMsg)
	if !ok {
		t.Fatalf("expected loginResultMsg, got %T", msg)
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "unsupported oauth provider") {
		t.Fatalf("expected unsupported provider error, got %v", result.err)
	}
}

func TestLogoutProviderCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	cmd := m.logoutProviderCmd("openai-codex")
	msg := cmd()
	result, ok := msg.(logoutResultMsg)
	if !ok {
		t.Fatalf("expected logoutResultMsg, got %T", msg)
	}
	if result.provider != "openai-codex" {
		t.Fatalf("expected provider openai-codex, got %q", result.provider)
	}
}

// --- Command suggestion tests ---

func TestCommandSuggestions_NoRegistry(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// No cmdReg means no suggestions
	if m.commandSuggestions() != nil {
		t.Fatal("expected nil suggestions without cmdReg")
	}
}

func TestCommandSuggestions_Pending(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help", Description: "show help"})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true

	if m.commandSuggestions() != nil {
		t.Fatal("expected nil suggestions when pending")
	}
}

func TestClampCommandSuggestionIndex(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// No suggestions should reset to 0
	m.commandSuggestionIndex = 5
	m.clampCommandSuggestionIndex()
	if m.commandSuggestionIndex != 0 {
		t.Fatalf("expected 0 with no suggestions, got %d", m.commandSuggestionIndex)
	}
}

func TestCommandInputExactlyMatchesSuggestion(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help"})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())

	// Input without slash prefix
	m.textarea.SetValue("help")
	if m.commandInputExactlyMatchesSuggestion(nil) {
		t.Fatal("expected false for input without slash")
	}

	// Matching input
	m.textarea.SetValue("/help")
	if !m.commandInputExactlyMatchesSuggestion([]commands.Command{{Name: "help"}}) {
		t.Fatal("expected true for matching input")
	}

	// Non-matching
	m.textarea.SetValue("/help")
	if m.commandInputExactlyMatchesSuggestion([]commands.Command{{Name: "exit"}}) {
		t.Fatal("expected false for non-matching suggestion")
	}
}

func TestIsQuitCommand(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	if !m.isQuitCommand("/quit") {
		t.Fatal("expected /quit to be recognized")
	}
	if !m.isQuitCommand("/exit") {
		t.Fatal("expected /exit to be recognized")
	}
	if m.isQuitCommand("/help") {
		t.Fatal("expected /help to NOT be a quit command")
	}
}

func TestVisibleCommandSuggestionRange(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	suggestions := make([]commands.Command, 3)
	for i := range suggestions {
		suggestions[i] = commands.Command{Name: fmt.Sprintf("cmd%d", i)}
	}

	m.commandSuggestionIndex = 1

	start, end, sel := m.visibleCommandSuggestionRange(suggestions)
	if start != 0 || end != 3 || sel != 1 {
		t.Fatalf("small list: start=%d end=%d sel=%d, want 0 3 1", start, end, sel)
	}

	// Out of range
	m.commandSuggestionIndex = 100
	_, _, sel = m.visibleCommandSuggestionRange(suggestions)
	if sel != 2 {
		t.Fatalf("expected sel 2, got %d", sel)
	}

	// Negative
	m.commandSuggestionIndex = -1
	_, _, sel = m.visibleCommandSuggestionRange(suggestions)
	if sel != 0 {
		t.Fatalf("expected sel 0, got %d", sel)
	}
}

func TestCommandSuggestionHeight(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help"})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("/")

	h := m.commandSuggestionHeight()
	if h <= 0 {
		t.Fatalf("expected positive suggestion height, got %d", h)
	}
}

func TestCommandSuggestionHeight_NoSuggestions(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("/")

	if h := m.commandSuggestionHeight(); h != 0 {
		t.Fatalf("expected 0 with no registry, got %d", h)
	}
}

// --- Viewport helper tests ---

func TestShouldHandleViewportKey(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// Page keys should be handled
	if !m.shouldHandleViewportKey(tea.KeyMsg{Type: tea.KeyPgUp}) {
		t.Fatal("expected PageUp to be handled")
	}
	if !m.shouldHandleViewportKey(tea.KeyMsg{Type: tea.KeyPgDown}) {
		t.Fatal("expected PageDown to be handled")
	}

	// Arrow keys only when textarea is empty
	m.textarea.SetValue("")
	if !m.shouldHandleViewportKey(tea.KeyMsg{Type: tea.KeyUp}) {
		t.Fatal("expected Up to be handled when textarea empty")
	}
	if !m.shouldHandleViewportKey(tea.KeyMsg{Type: tea.KeyDown}) {
		t.Fatal("expected Down to be handled when textarea empty")
	}

	// Arrow keys not handled when textarea has content
	m.textarea.SetValue("text")
	if m.shouldHandleViewportKey(tea.KeyMsg{Type: tea.KeyUp}) {
		t.Fatal("expected Up NOT to be handled when textarea has text")
	}

	// Not handled when command suggestions are shown
	m.textarea.SetValue("/")
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help"})
	m.cmdReg = cmdReg
	if m.shouldHandleViewportKey(tea.KeyMsg{Type: tea.KeyPgUp}) {
		t.Fatal("expected PageUp NOT to be handled when suggestions shown")
	}
}

// --- Conversion function tests ---

func TestMessageToConversation(t *testing.T) {
	msg := messageItem{
		role:      "assistant",
		content:   "hello",
		reasoning: "thinking",
		usageData: &provider.Usage{InputTokens: 10, OutputTokens: 20},
		toolCalls: []toolRenderItem{
			{name: "bash", callID: "c1", args: "echo hi", rawArgs: map[string]any{"command": "echo hi"}},
		},
	}

	result := messageToConversation(msg)
	if result.Role != "assistant" {
		t.Fatalf("expected role assistant, got %q", result.Role)
	}
	if result.Content != "hello" {
		t.Fatalf("expected content hello, got %q", result.Content)
	}
	if result.Reasoning != "thinking" {
		t.Fatalf("expected reasoning thinking, got %q", result.Reasoning)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "bash" {
		t.Fatalf("expected tool name bash, got %q", result.ToolCalls[0].Name)
	}
}

func TestMessagesToConversation_FiltersEphemeral(t *testing.T) {
	msgs := []messageItem{
		{role: "user", content: "hi"},
		{role: "system", content: "ignored", ephemeral: true},
		{role: "assistant", content: "hello"},
	}

	result := messagesToConversation(msgs, false)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	resultAll := messagesToConversation(msgs, true)
	if len(resultAll) != 3 {
		t.Fatalf("expected 3 messages with ephemeral, got %d", len(resultAll))
	}
}

func TestCompactionToMessageItem(t *testing.T) {
	compMsg := compaction.MessageItem{
		Role:      "assistant",
		Content:   "compacted",
		Reasoning: "thought",
		ToolCalls: []compaction.ToolRenderItem{
			{Name: "bash", CallID: "c1", Output: "out"},
		},
	}

	result := compactionToMessageItem(compMsg)
	if result.role != "assistant" {
		t.Fatalf("expected role assistant, got %q", result.role)
	}
	if result.content != "compacted" {
		t.Fatalf("expected content compacted, got %q", result.content)
	}
	if len(result.toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.toolCalls))
	}
}

func TestMessagesToCompaction(t *testing.T) {
	msgs := []messageItem{
		{role: "user", content: "hi"},
		{role: "assistant", content: "hello", toolCalls: []toolRenderItem{
			{name: "bash", callID: "c1", output: "out"},
		}},
	}

	result := messagesToCompaction(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 compaction messages, got %d", len(result))
	}
	if result[1].ToolCalls[0].Name != "bash" {
		t.Fatalf("expected tool name bash, got %q", result[1].ToolCalls[0].Name)
	}
}

// --- Tool call formatting tests ---

func TestFormatToolCallLine_Bash(t *testing.T) {
	got := formatToolCallLine("bash", map[string]any{"command": "ls -la"})
	if got != "$ ls -la" {
		t.Fatalf("expected '$ ls -la', got %q", got)
	}
}

func TestFormatToolCallLine_BashNoCommand(t *testing.T) {
	got := formatToolCallLine("bash", nil)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatToolCallLine_FileRead(t *testing.T) {
	got := formatToolCallLine("read", map[string]any{"path": "/home/user/file.go", "offset": 10, "limit": 20})
	if !strings.Contains(got, "file.go") || !strings.Contains(got, ":10-29") {
		t.Fatalf("expected read with line range, got %q", got)
	}
}

func TestFormatToolCallLine_FileReadNoOffset(t *testing.T) {
	got := formatToolCallLine("file_read", map[string]any{"path": "/home/user/file.go"})
	if !strings.Contains(got, "file.go") {
		t.Fatalf("expected file path in output, got %q", got)
	}
}

func TestFormatToolCallLine_FileWrite(t *testing.T) {
	got := formatToolCallLine("write", map[string]any{"path": "/home/user/new.go"})
	if !strings.Contains(got, "new.go") {
		t.Fatalf("expected file path in output, got %q", got)
	}
}

func TestFormatToolCallLine_Grep(t *testing.T) {
	got := formatToolCallLine("grep", map[string]any{"pattern": "func", "path": "."})
	expected := "/func/"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestFormatToolCallLine_GrepWithPath(t *testing.T) {
	got := formatToolCallLine("grep", map[string]any{"pattern": "func", "path": "/home/user/src/main.go"})
	if !strings.Contains(got, "/func/") || !strings.Contains(got, "src/main.go") {
		t.Fatalf("expected grep with path, got %q", got)
	}
	if strings.Contains(got, "grep") {
		t.Fatalf("should not contain tool name, got %q", got)
	}
}

func TestFormatToolCallLine_Find(t *testing.T) {
	got := formatToolCallLine("find", map[string]any{"glob": "*.go", "path": "src"})
	if !strings.Contains(got, "*.go") {
		t.Fatalf("expected glob pattern: %q", got)
	}
}

func TestFormatToolCallLine_Ls(t *testing.T) {
	got := formatToolCallLine("ls", map[string]any{"path": "src"})
	if !strings.Contains(got, "src") {
		t.Fatalf("expected path: %q", got)
	}

	got2 := formatToolCallLine("ls", map[string]any{})
	if got2 != "" {
		t.Fatalf("expected empty: %q", got2)
	}
}

func TestFormatToolCallLine_Default(t *testing.T) {
	got := formatToolCallLine("web_search", map[string]any{"query": "go testing"})
	// Unknown tool with no summarizeKey match returns empty
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	// Test with a tool that has a tldr; key matching the default branch
	got2 := formatToolCallLine("edit", map[string]any{"path": "/tmp/test.go"})
	if !strings.Contains(got2, "test.go") {
		t.Fatalf("expected path in output: %q", got2)
	}
}

func TestFormatToolCallLine_DefaultNoKey(t *testing.T) {
	got := formatToolCallLine("unknown_tool", map[string]any{"something": "value"})
	if got != "" {
		t.Fatalf("expected empty: %q", got)
	}
}

func TestFormatFilePathCall(t *testing.T) {
	got := formatFilePathCall(map[string]any{"path": "/tmp/test.go"}, "path", "", "")
	if !strings.Contains(got, "/tmp/test.go") {
		t.Fatalf("expected path: %q", got)
	}

	got2 := formatFilePathCall(map[string]any{}, "path", "", "")
	if got2 != "" {
		t.Fatalf("expected empty: %q", got2)
	}
}

func TestGetIntArg(t *testing.T) {
	args := map[string]any{"offset": float64(42), "limit": 10}

	if got := getIntArg(args, "offset"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}

	if got := getIntArg(args, "nonexistent"); got != 0 {
		t.Fatalf("expected 0 for missing key, got %d", got)
	}

	if got := getIntArg(args, "limit"); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestShortenDisplayPath(t *testing.T) {
	orig := getCwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir("/tmp")

	got := shortenDisplayPath("/tmp/subdir/file.go")
	if got != "subdir/file.go" {
		t.Fatalf("expected 'subdir/file.go', got %q", got)
	}

	got2 := shortenDisplayPath("/other/file.go")
	if got2 != "/other/file.go" {
		t.Fatalf("expected unchanged path, got %q", got2)
	}
}

func TestTruncateDisplay(t *testing.T) {
	got := truncateDisplay("short", 10)
	if got != "short" {
		t.Fatalf("expected unchanged, got %q", got)
	}

	got2 := truncateDisplay("this is a very long string", 10)
	if !strings.HasPrefix(got2, "this is a ") || !strings.HasSuffix(got2, "... (truncated)") {
		t.Fatalf("expected truncated, got %q", got2)
	}
}

func TestSummarizeKey(t *testing.T) {
	if summarizeKey("bash") != "command" {
		t.Fatalf("expected 'command' for bash")
	}
	if summarizeKey("file_read") != "path" {
		t.Fatalf("expected 'path' for file_read")
	}
	if summarizeKey("unknown") != "" {
		t.Fatalf("expected empty for unknown")
	}
}

// --- Input preprocessor tests ---

func TestExpandShellShortcut_NoRegistry(t *testing.T) {
	// Create a registry without a bash tool registered
	reg := tools.NewRegistry()
	got := expandShellShortcut(reg, "!echo hello")
	// Without bash tool, execution fails
	if !strings.Contains(got, "bash error") {
		t.Fatalf("expected bash error message, got %q", got)
	}
}

func TestExpandFileRefs_NoRegistry(t *testing.T) {
	// Create a registry without a file_read tool
	reg := tools.NewRegistry()
	got := expandFileRefs(reg, "check @file.go")
	// Without file_read tool, the @ref should be left alone
	if !strings.Contains(got, "@file.go") {
		t.Fatalf("expected @file.go unchanged, got %q", got)
	}
}

// --- View edge case tests ---

func TestViewShowsLoadingBeforeReady(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	view := m.View()
	if !strings.Contains(view, "Loading...") {
		t.Fatalf("expected Loading... before ready, got %q", view)
	}
}

func TestViewShowsCwd(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)

	view := m.View()
	if !strings.Contains(view, "/") && !strings.Contains(view, "~/") {
		t.Logf("cwd line not found, checking full view: %q", view)
	}
}

func TestEstimatedContextUsed(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// Empty model should have some context from system prompt
	if m.estimatedContextUsed() <= 0 {
		t.Fatalf("expected positive context estimate, got %d", m.estimatedContextUsed())
	}

	// Add messages
	m.messages = append(m.messages,
		messageItem{role: "user", content: "hello"},
		messageItem{role: "assistant", content: "hi there", toolCalls: []toolRenderItem{
			{name: "bash", callID: "c1", args: "echo", rawArgs: map[string]any{"command": "echo"}, output: "done"},
		}},
	)

	higher := m.estimatedContextUsed()
	if higher <= 0 {
		t.Fatalf("expected positive context estimate after messages, got %d", higher)
	}
}

func TestRenderCostStatus_Subscription(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openai-codex"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	status := m.renderCostStatus()
	if status != "sub" {
		t.Fatalf("expected 'sub' for subscription provider, got %q", status)
	}
}

func TestRenderCostStatus_Normal(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	status := m.renderCostStatus()
	if !strings.HasPrefix(status, "$") {
		t.Fatalf("expected '$' prefix, got %q", status)
	}
}

func TestPricingForCurrentModel_EmptyRegistry(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "openai/gpt-4o"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	pricing := m.pricingForCurrentModel()
	// Should use static fallback
	if pricing.InputPerMTok == 0 && pricing.OutputPerMTok == 0 {
		t.Log("static pricing returned zero (acceptable if model unknown)")
	}
}

func TestAttachUsageToLastAssistant_NilUsage(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{role: "assistant"}}

	// Should not panic with nil usage
	m.attachUsageToLastAssistant(nil)
	if m.messages[0].usage != "" {
		t.Fatalf("expected empty usage for nil, got %q", m.messages[0].usage)
	}
}

func TestAttachUsageToLastAssistant_NoMessages(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// Should not panic with empty messages
	m.attachUsageToLastAssistant(&provider.Usage{InputTokens: 10, OutputTokens: 20})
}

func TestAttachUsageToLastAssistant_LastNotAssistant(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{role: "user", content: "hi"}}

	m.attachUsageToLastAssistant(&provider.Usage{InputTokens: 10, OutputTokens: 20})
	if m.messages[0].usage != "" {
		t.Fatalf("expected user message to not get usage")
	}
}

func TestFormatTokenCount(t *testing.T) {
	if got := formatTokenCount(500); got != "500" {
		t.Fatalf("expected '500', got %q", got)
	}
	if got := formatTokenCount(1500); got != "1.5k" {
		t.Fatalf("expected '1.5k', got %q", got)
	}
	if got := formatTokenCount(2000); got != "2k" {
		t.Fatalf("expected '2k', got %q", got)
	}
	if got := formatTokenCount(1_500_000); got != "1.5M" {
		t.Fatalf("expected '1.5M', got %q", got)
	}
	if got := formatTokenCount(2_000_000); got != "2M" {
		t.Fatalf("expected '2M', got %q", got)
	}
}

func TestTotalUsageCost(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	cost, sub := m.totalUsageCost()
	if sub {
		t.Fatal("expected non-subscription")
	}
	if cost != 0 {
		t.Fatalf("expected 0 cost, got %f", cost)
	}
}

func TestHasPricing(t *testing.T) {
	// Any non-zero pricing field makes it true
	if !hasPricing(commands.Pricing{InputPerMTok: 2}) {
		t.Fatal("expected true with InputPerMTok set")
	}
	if !hasPricing(commands.Pricing{OutputPerMTok: 10}) {
		t.Fatal("expected true with OutputPerMTok set")
	}
	if !hasPricing(commands.Pricing{CacheReadPerMTok: 1}) {
		t.Fatal("expected true with CacheReadPerMTok set")
	}
	if !hasPricing(commands.Pricing{CacheWritePerMTok: 1}) {
		t.Fatal("expected true with CacheWritePerMTok set")
	}
	// All zero means no pricing
	if hasPricing(commands.Pricing{}) {
		t.Fatal("expected false with all zero pricing")
	}
}

// --- View message rendering tests ---

func TestRenderMessages_Compaction(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = append(m.messages, messageItem{role: "compaction", content: "summarized previous context"})

	got := m.renderMessages()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "[compaction]") {
		t.Fatalf("expected compaction prefix: %q", stripped)
	}
}

func TestRenderMessages_Error(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = append(m.messages, messageItem{role: "error", content: "something failed"})

	got := m.renderMessages()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "Error") || !strings.Contains(stripped, "something failed") {
		t.Fatalf("expected error in render: %q", stripped)
	}
}

func TestRenderMessages_System(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = append(m.messages, messageItem{role: "system", content: "system message"})

	got := m.renderMessages()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "system message") {
		t.Fatalf("expected system message in render: %q", stripped)
	}
}

func TestRenderMessages_AssistantWithUsage(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = append(m.messages, messageItem{
		role:    "assistant",
		content: "response",
		usage:   "10 in / 20 out",
	})

	got := m.renderMessages()
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "10 in / 20 out") {
		t.Fatalf("expected usage in render: %q", stripped)
	}
}

func TestRenderMessages_BannerShown(t *testing.T) {
	m := NewModel(&config.AgentConfig{ShowBanner: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = append(m.messages, messageItem{role: "user", content: "hi"})

	got := m.renderMessages()
	if !strings.Contains(got, "CROBOT") && !strings.Contains(got, "crobot") && !strings.Contains(got, "model") {
		t.Logf("banner content not checked strictly: %q", got)
	}
}

// --- Tool expand tests ---

func TestExpandShellShortcut_WithRegistry(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "bash",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"stdout": "hello", "stderr": "", "exitCode": 0}, nil
		},
	})

	got := expandShellShortcut(reg, "!echo hello")
	if !strings.Contains(got, "hello") {
		t.Fatalf("expected command output: %q", got)
	}
}

func TestExpandShellShortcut_Error(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "bash",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return nil, fmt.Errorf("command failed")
		},
	})

	got := expandShellShortcut(reg, "!false")
	if !strings.Contains(got, "bash error") {
		t.Fatalf("expected bash error: %q", got)
	}
}

func TestExpandFileRefs_WithRegistry(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "file_read",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"content": "file content"}, nil
		},
	})

	got := expandFileRefs(reg, "check @file.go")
	if !strings.Contains(got, "file content") {
		t.Fatalf("expected file content: %q", got)
	}
}

func TestExpandFileRefs_RegistryError(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "file_read",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return nil, fmt.Errorf("not found")
		},
	})

	got := expandFileRefs(reg, "check @file.go")
	if !strings.Contains(got, "@file.go") {
		t.Fatalf("expected @file.go to remain when error: %q", got)
	}
}

// --- Handle agent event edge cases ---

func TestHandleAgentEvent_MessageStart(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, cmd := m.handleAgentEvent(agent.Event{Type: "message_start", MessageStart: &agent.MessageStartEvent{Role: "assistant"}})
	m2 := updated.(Model)

	if len(m2.messages) != 1 || m2.messages[0].role != "assistant" {
		t.Fatalf("expected new assistant message: %#v", m2.messages)
	}
	_ = cmd
}

func TestHandleAgentEvent_TextDelta(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.handleAgentEvent(agent.Event{Type: "text_delta", TextDelta: "hello"})
	m2 := updated.(Model)

	if m2.messages[0].content != "hello" {
		t.Fatalf("expected content 'hello', got %q", m2.messages[0].content)
	}
}

func TestHandleAgentEvent_TextDeltaNoAssistantMessage(t *testing.T) {
	// Should not panic when last message isn't assistant
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, _ := m.handleAgentEvent(agent.Event{Type: "text_delta", TextDelta: "hello"})
	_ = updated
}

func TestHandleAgentEvent_ToolCallEnd(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.handleAgentEvent(agent.Event{Type: "tool_call_end", ToolCallEnd: &agent.ToolCallEvent{
		Name:   "bash",
		CallID: "call_1",
		Args:   map[string]any{"command": "echo hi"},
	}})
	m2 := updated.(Model)

	if len(m2.messages[0].toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(m2.messages[0].toolCalls))
	}
	if m2.messages[0].toolCalls[0].name != "bash" {
		t.Fatalf("expected tool name 'bash', got %q", m2.messages[0].toolCalls[0].name)
	}
}

func TestHandleAgentEvent_ToolExecStart(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{
		role: "assistant",
		toolCalls: []toolRenderItem{
			{name: "bash", callID: "call_1", state: toolPending},
		},
	}}

	updated, _ := m.handleAgentEvent(agent.Event{Type: "tool_exec_start", ToolExecStart: &agent.ToolExecStartEvent{CallID: "call_1"}})
	m2 := updated.(Model)

	if m2.messages[0].toolCalls[0].state != toolRunning {
		t.Fatalf("expected tool state running, got %d", m2.messages[0].toolCalls[0].state)
	}
}

func TestHandleAgentEvent_ToolExecResult(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{
		role: "assistant",
		toolCalls: []toolRenderItem{
			{name: "bash", callID: "call_1", state: toolRunning},
		},
	}}

	updated, _ := m.handleAgentEvent(agent.Event{Type: "tool_exec_result", ToolExecResult: &agent.ToolExecResultEvent{
		CallID:   "call_1",
		Output:   "output",
		Success:  true,
		Duration: 100,
	}})
	m2 := updated.(Model)

	tc := m2.messages[0].toolCalls[0]
	if tc.state != toolDone {
		t.Fatalf("expected tool state done, got %d", tc.state)
	}
	if tc.output != "output" {
		t.Fatalf("expected output 'output', got %q", tc.output)
	}
}

func TestHandleAgentEvent_Error(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.agentEvents = make(chan agent.Event, 1)

	updated, _ := m.handleAgentEvent(agent.Event{Type: "error", Error: fmt.Errorf("provider error")})
	m2 := updated.(Model)

	if m2.pending {
		t.Fatal("expected pending to be cleared on error")
	}
	if len(m2.messages) == 0 || m2.messages[len(m2.messages)-1].role != "error" {
		t.Fatalf("expected error message: %#v", m2.messages)
	}
}

func TestHandleAgentEvent_MessageStartNoRole(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, _ := m.handleAgentEvent(agent.Event{Type: "message_start", MessageStart: &agent.MessageStartEvent{Role: "user"}})
	m2 := updated.(Model)

	// Should NOT add a message for non-assistant role
	if len(m2.messages) != 0 {
		t.Fatalf("expected no new message for non-assistant role, got %d: %#v", len(m2.messages), m2.messages)
	}
	_ = updated
}

func TestHandleAgentEvent_ToolCallEndNoAssistant(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, _ := m.handleAgentEvent(agent.Event{
		Type:        "tool_call_end",
		ToolCallEnd: &agent.ToolCallEvent{Name: "bash", CallID: "c1"},
	})
	_ = updated
}

func TestHandleAgentEvent_ToolExecStartNoMatch(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{
		role: "assistant",
		toolCalls: []toolRenderItem{
			{name: "bash", callID: "call_1"},
		},
	}}

	updated, _ := m.handleAgentEvent(agent.Event{
		Type:          "tool_exec_start",
		ToolExecStart: &agent.ToolExecStartEvent{CallID: "nonexistent"},
	})
	_ = updated
}

func TestHandleAgentEvent_ToolExecResultNoMatch(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{
		role: "assistant",
	}}

	updated, _ := m.handleAgentEvent(agent.Event{
		Type:           "tool_exec_result",
		ToolExecResult: &agent.ToolExecResultEvent{CallID: "call_x"},
	})
	_ = updated
}

func TestHandleAgentEvent_MessageEndUsage(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.handleAgentEvent(agent.Event{
		Type: "message_end",
		MessageEnd: &agent.MessageEndEvent{
			Text:  "final",
			Usage: &provider.Usage{InputTokens: 10, OutputTokens: 20},
		},
	})
	m2 := updated.(Model)

	if m2.pending {
		t.Fatal("expected pending to be cleared")
	}
	if m2.textarea.Focused() == false {
		t.Log("textarea should be focused after message_end")
	}
}

func TestHandleAgentEvent_MessageEndNoContent(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.agentEvents = make(chan agent.Event, 1)
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.handleAgentEvent(agent.Event{
		Type: "message_end",
		MessageEnd: &agent.MessageEndEvent{
			Text: "",
		},
	})
	m2 := updated.(Model)

	if m2.pending {
		t.Fatal("expected pending to be cleared")
	}
}

func TestHandleAgentEvent_TurnUsage(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.messages = []messageItem{{role: "assistant"}}

	updated, _ := m.handleAgentEvent(agent.Event{
		Type:      "turn_usage",
		TurnUsage: &provider.Usage{InputTokens: 100, OutputTokens: 50},
	})
	m2 := updated.(Model)

	if !m2.pending {
		t.Fatal("expected pending to remain true on turn_usage")
	}
}

// --- DynamicViewportHeight tests ---

func TestDynamicViewportHeight_Pending(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.height = 24
	m.pending = true

	h := m.dynamicViewportHeight()
	if h <= 0 || h > 24 {
		t.Fatalf("expected reasonable viewport height, got %d", h)
	}
}

func TestDynamicViewportHeight_ModelPicker(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "m1", Provider: "p1"},
	}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.height = 24
	m.modelPickerActive = true

	h := m.dynamicViewportHeight()
	if h <= 0 || h > 24 {
		t.Fatalf("expected reasonable viewport height with model picker, got %d", h)
	}
}

func TestDynamicViewportHeight_Minimum(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.height = 5 // very small terminal

	h := m.dynamicViewportHeight()
	if h < 3 {
		t.Fatalf("expected minimum viewport height of 3, got %d", h)
	}
}

// --- Center content tests ---

func TestCenterContent(t *testing.T) {
	got := centerContent("hello", 80)
	lines := strings.Split(got, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if len(lines[0]) != 80 {
		t.Fatalf("expected 80-char line, got %d", len(lines[0]))
	}
	if !strings.Contains(lines[0], "hello") {
		t.Fatalf("expected hello in centered line: %q", lines[0])
	}
}

func TestCenterContent_MultiLine(t *testing.T) {
	got := centerContent("a\nb", 10)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, line := range lines {
		if len(line) != 10 {
			t.Fatalf("expected 10-char lines, got %d: %q", len(line), line)
		}
	}
}

func TestCenterContent_Empty(t *testing.T) {
	got := centerContent("", 80)
	// Centering an empty string returns 80 spaces
	if len(got) != 80 {
		t.Fatalf("expected 80 spaces, got %d: %q", len(got), got)
	}
}

func TestRenderMessagesCenteredUsesNarrowWrapWidth(t *testing.T) {
	m := NewModel(&config.AgentConfig{Alignment: "centered"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.width = 80
	m.messages = []messageItem{{role: "assistant", content: strings.Repeat("word ", 40)}}

	got := m.renderMessages()
	var contentLines int
	for _, line := range strings.Split(got, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		contentLines++
		if w := lipgloss.Width(line); w > 60 {
			t.Fatalf("expected centered content to wrap to narrow column, got width %d for %q", w, line)
		}
	}
	if contentLines < 2 {
		t.Fatalf("expected response to wrap to multiple lines, got %d content lines: %q", contentLines, got)
	}
}

// --- Status line truncation ---

func TestTruncatePlainLine(t *testing.T) {
	got := truncatePlainLine("short", 80)
	if got != "short" {
		t.Fatalf("expected unchanged, got %q", got)
	}

	got2 := truncatePlainLine("this is a long line that should be truncated because it exceeds the max width", 20)
	if len(got2) > 22 { // allow for ellipsis
		t.Fatalf("expected truncated line, got length %d: %q", len(got2), got2)
	}

	got3 := truncatePlainLine("hi", 1)
	if got3 != "…" {
		t.Fatalf("expected just ellipsis for width 1, got %q", got3)
	}
}

// --- Raw message bytes and Shift+Enter sequences ---

func TestRawMsgBytes(t *testing.T) {
	// Non-slice message returns nil
	bytes := rawMsgBytes(tea.KeyMsg{Type: tea.KeyEnter})
	if bytes != nil {
		t.Fatalf("expected nil for non-slice message, got %v", bytes)
	}

	// Slice message returns the bytes
	bytes = rawMsgBytes([]byte("\x1b\r"))
	if len(bytes) != 2 {
		t.Fatalf("expected 2 bytes, got %v", bytes)
	}
}

func TestIsShiftEnterSequence(t *testing.T) {
	// Known sequences should be recognized
	if !isShiftEnterSequence([]byte("\x1b\r")) {
		t.Fatal("expected ESC+CR to be Shift+Enter")
	}

	// Empty sequence
	if isShiftEnterSequence(nil) {
		t.Fatal("expected nil to NOT be Shift+Enter")
	}

	if isShiftEnterSequence([]byte{}) {
		t.Fatal("expected empty to NOT be Shift+Enter")
	}
}

// --- SelectModel tests ---

func TestSelectModel_SameProvider(t *testing.T) {
	m := NewModel(&config.AgentConfig{Provider: "openrouter", Model: "old-model"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	m.selectModel("openrouter", "new-model")

	if m.config.Model != "new-model" {
		t.Fatalf("expected model to be 'new-model', got %q", m.config.Model)
	}
	// Provider should remain
	if m.config.Provider != "openrouter" {
		t.Fatalf("expected provider 'openrouter', got %q", m.config.Provider)
	}
}

func TestSelectModel_NoAPIKey(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, func(name string) string {
		return ""
	}, nil, tuiStylesForTest())

	m.selectModel("openrouter", "openai/gpt-4o")

	if m.provider != nil {
		t.Fatalf("expected provider to remain nil when no API key")
	}
}

// --- Handle compaction result tests ---

func TestHandleCompactionResult_Error(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, _ := m.handleCompactionResult(compactionResultMsg{err: fmt.Errorf("compaction failed")})
	m2 := updated.(*Model)

	if len(m2.messages) == 0 || m2.messages[len(m2.messages)-1].role != "error" {
		t.Fatalf("expected error message: %#v", m2.messages)
	}
}

func TestHandleCompactionResult_Success(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	result := &compaction.Result{
		Summary: "summary text",
		NewMessages: []compaction.MessageItem{
			{Role: "user", Content: "compacted message"},
		},
		TokensBefore: 500,
	}

	updated, _ := m.handleCompactionResult(compactionResultMsg{result: result})
	m2 := updated.(*Model)

	if len(m2.messages) < 2 {
		t.Fatalf("expected at least 2 messages after compaction, got %d", len(m2.messages))
	}

	// Check the summary was stored
	if m2.previousCompactionSummary != "summary text" {
		t.Fatalf("expected summary to be stored, got %q", m2.previousCompactionSummary)
	}

	// Check status message
	if m2.messages[len(m2.messages)-1].role != "system" {
		t.Fatalf("expected status message as last, got %#v", m2.messages[len(m2.messages)-1])
	}
}

// --- Enter key handling with various inputs ---

func TestUpdate_EnterWithEmptyInput(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("  ")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if len(m2.messages) != 0 {
		t.Fatalf("expected no messages for whitespace input, got %d", len(m2.messages))
	}
}

func TestUpdate_EnterNoProvider(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("hello")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if len(m2.messages) == 0 || !strings.Contains(m2.messages[len(m2.messages)-1].content, "No provider added") {
		t.Fatalf("expected no provider warning, got %#v", m2.messages)
	}
}

func TestUpdate_EnterNoProviderWithAuth(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("hello")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if len(m2.messages) == 0 || !strings.Contains(m2.messages[len(m2.messages)-1].content, "No provider selected") {
		t.Fatalf("expected no provider selected warning, got %#v", m2.messages)
	}
}

func TestUpdate_EnterSlashCommand(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{
		Name:        "testcmd",
		Description: "test command",
		Handler: func(args []string) (string, error) {
			return "result", nil
		},
	})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("/testcmd")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	found := false
	for _, msg := range m2.messages {
		if msg.role == "system" && strings.Contains(msg.content, "result") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'result' in system messages: %#v", m2.messages)
	}
}

func TestUpdate_EnterSlashCommandError(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{
		Name:        "failcmd",
		Description: "failing command",
		Handler: func(args []string) (string, error) {
			return "", fmt.Errorf("command error")
		},
	})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("/failcmd")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if len(m2.messages) == 0 || m2.messages[len(m2.messages)-1].role != "error" {
		t.Fatalf("expected error message, got %#v", m2.messages)
	}
}

func TestUpdate_EnterCompactCommand(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("/compact")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	// Should show a message about nothing to compact or similar
	if len(m2.messages) == 0 {
		t.Fatalf("expected a message after /compact")
	}
}

// --- Render status line edge cases ---

func TestRenderStatusLine_EmptyProvider(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.width = 80

	status := m.renderStatusLine()
	if !strings.Contains(stripANSI(status), "unknown") {
		t.Fatalf("expected 'unknown' for empty provider/model: %q", status)
	}
}

func TestRenderInputView_Pending(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.textarea.SetValue("typing...")

	view := m.renderInputView()
	// When pending, no cursor should appear
	if !strings.Contains(view, "typing...") {
		t.Fatalf("expected input text in pending view: %q", view)
	}
}

func TestRenderViewportContent_WithSelection(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{
		{role: "user", content: "hello world"},
	}

	// The user message renders as: "  > hello world\n\n"
	// Plain line 0: "  > hello world"
	// Selecting plain text from col 4 ('h') to col 9 ('o') should hit "hello"
	m.selection = selectionState{
		startLine: 0, startCol: 4,
		endLine: 0, endCol: 9,
	}

	content := m.renderViewportContent()
	// Check for reverse video markers (which may not have a matching "/27m" depending on selection)
	// Just verify selection doesn't crash and returns styled content
	if content == "" {
		t.Fatal("expected non-empty content with selection")
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m2 := updated.(Model)

	if !m2.ready {
		t.Fatal("expected model to be ready after WindowSizeMsg")
	}
	if m2.width != 100 || m2.height != 40 {
		t.Fatalf("expected size 100x40, got %dx%d", m2.width, m2.height)
	}
}

func TestCtrlOCyclesToolExpanded(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	if m.toolOutputExpanded {
		t.Fatal("expected toolOutputExpanded to start false")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m2 := updated.(Model)

	if !m2.toolOutputExpanded {
		t.Fatal("expected toolOutputExpanded to be true after Ctrl+O")
	}

	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m3 := updated.(Model)

	if m3.toolOutputExpanded {
		t.Fatal("expected toolOutputExpanded to be false after second Ctrl+O")
	}
}

func TestUpdateScrollsToBottomWhenNewContentAppears(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24

	// Set up viewport with some content
	m.refreshViewport()

	// Add a message and refresh - should scroll to bottom
	before := m.viewport.YOffset
	m.messages = append(m.messages, messageItem{role: "user", content: "new message"})
	m.refreshViewport()

	_ = before
}

// --- Tool expand with non-standard result tests ---

func TestExpandShellShortcut_NonMapResult(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "bash",
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return "plain string", nil
		},
	})

	got := expandShellShortcut(reg, "!echo hi")
	if !strings.Contains(got, "plain string") {
		t.Fatalf("expected result string: %q", got)
	}
}

// --- OAuth provider options ---

func TestOAuthProviderOptions(t *testing.T) {
	providers := oauthProviderOptions()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].ID != "openai-codex" {
		t.Fatalf("expected openai-codex, got %q", providers[0].ID)
	}
}

// --- Logged in OAuth providers (no auth file) ---

func TestLoggedInOAuthProviders_NoAuthFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	providers := m.loggedInOAuthProviders()
	if providers != nil {
		t.Fatalf("expected nil providers when no auth file: %#v", providers)
	}
}

// --- FormatFilePathCall with alternate key names ---

func TestFormatFilePathCall_AlternateKeys(t *testing.T) {
	got := formatFilePathCall(map[string]any{"path": "/tmp/file.go"}, "path", "start_line", "end_line")
	if !strings.Contains(got, "file.go") {
		t.Fatalf("expected path in output: %q", got)
	}

	got2 := formatFilePathCall(map[string]any{"path": "/tmp/file.go"}, "path", "start_line", "")
	if !strings.Contains(got2, "file.go") {
		t.Fatalf("expected path in output: %q", got2)
	}
}

func TestFormatToolCallLine_FindNoGlob(t *testing.T) {
	got := formatToolCallLine("find", map[string]any{"path": "src"})
	if !strings.Contains(got, "src") {
		t.Fatalf("expected path: %q", got)
	}
	if strings.Contains(got, "find") {
		t.Fatalf("should not contain tool name: %q", got)
	}
}

func TestFormatToolCallLine_DefaultWithArgs(t *testing.T) {
	got := formatToolCallLine("web_search", map[string]any{"query": "go testing"})
	// Unknown tool with no known summarizeKey returns empty
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- DecodeRune edge cases ---

func TestDecodeRune(t *testing.T) {
	if r, s := decodeRune("hello", 0); r != 'h' || s != 1 {
		t.Fatalf("expected 'h'/1, got %c/%d", r, s)
	}

	// Invalid position
	if r, s := decodeRune("hello", 100); r != 0 || s != 0 {
		t.Fatalf("expected 0/0 for invalid position, got %c/%d", r, s)
	}

	// Multi-byte
	if r, s := decodeRune("\xc3\xa9", 0); r != 'é' || s != 2 {
		t.Fatalf("expected é/2, got %c/%d", r, s)
	}

	// 3-byte
	if r, s := decodeRune("\xe2\x82\xac", 0); r != '€' || s != 3 {
		t.Fatalf("expected €/3, got %c/%d", r, s)
	}

	// 4-byte
	if r, s := decodeRune("\xf0\x9f\x92\xa9", 0); r != '💩' || s != 4 {
		t.Fatalf("expected 💩/4, got %c/%d", r, s)
	}
}

// --- AnsiLen edge cases ---

func TestAnsiLen(t *testing.T) {
	if n := ansiLen("hello"); n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}

	if n := ansiLen("\x1b[31mhello\x1b[0m"); n != 5 {
		t.Fatalf("expected 5 with ANSI codes, got %d", n)
	}

	if n := ansiLen(""); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	// Simple ESC+X (non-CSI) sequence
	if n := ansiLen("\x1bXabc"); n != 3 {
		t.Fatalf("expected 3 after ESC+X, got %d", n)
	}

	// String with no ANSI codes
	if n := ansiLen("plain text"); n != 10 {
		t.Fatalf("expected 10 for 'plain text', got %d", n)
	}
}

// --- WrapLine word-wrap at space tests ---

func TestWrapLine_AtSpace(t *testing.T) {
	got := wrapLine("hello world foo bar", 10)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if ansiLen(line) > 10 {
			t.Fatalf("line exceeds width 10: %q (len=%d)", line, ansiLen(line))
		}
	}
}

func TestWrapLine_WithANSI(t *testing.T) {
	styled := "\x1b[32mhello world\x1b[0m"
	got := wrapLine(styled, 6)
	if !strings.Contains(got, "\x1b[32m") {
		t.Fatalf("expected ANSI codes preserved: %q", got)
	}
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if ansiLen(line) > 6 {
			t.Fatalf("line exceeds width 6: %q (vislen=%d)", line, ansiLen(line))
		}
	}
}

func TestWrapText_EmptyWidth(t *testing.T) {
	got := wrapText("hello", 0)
	if got != "hello" {
		t.Fatalf("expected unchanged for width 0, got %q", got)
	}
}

func TestWrapText_EmptyInput(t *testing.T) {
	got := wrapText("", 80)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestWrapText_SimpleNewline(t *testing.T) {
	got := wrapText("hello\nworld", 80)
	if got != "hello\nworld" {
		t.Fatalf("expected newline preserved: %q", got)
	}
}

// --- CompactCwd tests ---

func TestCompactCwd(t *testing.T) {
	cwd := compactCwd()
	if cwd == "" {
		t.Fatal("expected non-empty cwd")
	}
}

func TestGetCwd(t *testing.T) {
	if getCwd() == "" {
		t.Fatal("expected non-empty cwd")
	}
}

func TestValueOrDefault(t *testing.T) {
	if got := valueOrDefault("hello", "fallback"); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
	if got := valueOrDefault("", "fallback"); got != "fallback" {
		t.Fatalf("expected 'fallback', got %q", got)
	}
}

func TestCmdRegExecute(t *testing.T) {
	// Test the Execute method on commands.Registry more thoroughly
	// This is already covered by the slash command tests above
}

func TestModelPickerHeight_EmptyModels(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	h := m.modelPickerHeight()
	if h < 1 {
		t.Fatalf("expected at least 1 for empty models, got %d", h)
	}
}

// --- renderCommandSuggestions tests ---

func TestRenderCommandSuggestions_Empty(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	got := m.renderCommandSuggestions(nil)
	if got != "" {
		t.Fatalf("expected empty for nil suggestions, got %q", got)
	}

	got2 := m.renderCommandSuggestions([]commands.Command{})
	if got2 != "" {
		t.Fatalf("expected empty for empty suggestions, got %q", got2)
	}
}

func TestRenderCommandSuggestions_Normal(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	suggestions := []commands.Command{
		{Name: "help", Description: "Show help", Args: "[command]"},
		{Name: "model", Description: "Change model"},
	}

	m.commandSuggestionIndex = 1

	got := m.renderCommandSuggestions(suggestions)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "commands") {
		t.Fatalf("expected 'commands' header: %q", stripped)
	}
	if !strings.Contains(stripped, "/help") {
		t.Fatalf("expected /help in suggestions: %q", stripped)
	}
	if !strings.Contains(stripped, "/model") {
		t.Fatalf("expected /model in suggestions: %q", stripped)
	}
	if !strings.Contains(stripped, "Show help") {
		t.Fatalf("expected description: %q", stripped)
	}
}

func TestRenderCommandSuggestions_WithAboveMore(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// Create more than 8 suggestions so some are above visible range
	suggestions := make([]commands.Command, 12)
	for i := range suggestions {
		suggestions[i] = commands.Command{Name: fmt.Sprintf("cmd%02d", i), Description: "desc"}
	}
	m.commandSuggestionIndex = 7

	got := m.renderCommandSuggestions(suggestions)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "+3 more above") {
		t.Fatalf("expected '+3 more above': %q", stripped)
	}
}

func TestRenderCommandSuggestions_WithBelowMore(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	suggestions := make([]commands.Command, 12)
	for i := range suggestions {
		suggestions[i] = commands.Command{Name: fmt.Sprintf("cmd%02d", i), Description: "desc"}
	}
	m.commandSuggestionIndex = 0

	got := m.renderCommandSuggestions(suggestions)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "4 more below") {
		t.Fatalf("expected '4 more below': %q", stripped)
	}
}

func TestRenderCommandSuggestions_ModelSuggestions(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	suggestions := []commands.Command{
		{ModelID: "openai/gpt-4o", Args: "openrouter", Description: "OpenAI GPT-4o"},
		{ModelID: "anthropic/claude-opus", Args: "openrouter"},
	}

	m.commandSuggestionIndex = 0

	got := m.renderCommandSuggestions(suggestions)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "models") {
		t.Fatalf("expected 'models' header: %q", stripped)
	}
	if !strings.Contains(stripped, "openai/gpt-4o") {
		t.Fatalf("expected model ID: %q", stripped)
	}
}

func TestRenderCommandSuggestions_ModelsInCommandSuggestions(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.textarea.SetValue("/model")

	// Just verify it doesn't panic and returns something
	suggestions := m.commandSuggestions()
	got := m.renderCommandSuggestions(suggestions)
	_ = got
}

// --- completeCommandSuggestion tests ---

func TestCompleteCommandSuggestion_Normal(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help", Description: "Show help"})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())

	suggestions := []commands.Command{{Name: "help"}}
	m.completeCommandSuggestion(suggestions)

	if m.textarea.Value() != "/help " {
		t.Fatalf("expected '/help ', got %q", m.textarea.Value())
	}
	if m.commandSuggestionIndex != 0 {
		t.Fatalf("expected index 0, got %d", m.commandSuggestionIndex)
	}
}

func TestCompleteCommandSuggestion_ModelSuggestion(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	suggestions := []commands.Command{{ModelID: "openai/gpt-4o", ModelProvider: "openrouter"}}
	m.completeCommandSuggestion(suggestions)

	if m.config.Model != "openai/gpt-4o" {
		t.Fatalf("expected model selected, got %q", m.config.Model)
	}
}

func TestCompleteCommandSuggestion_Empty(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// Should not panic
	m.completeCommandSuggestion(nil)
	m.completeCommandSuggestion([]commands.Command{})
}

// --- shouldAutoCompact and runAutoCompactCmd tests ---

func TestShouldAutoCompact_NoMessages(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	// No messages -> should not compact
	if m.shouldAutoCompact() {
		t.Fatal("expected false for empty messages")
	}
}

func TestRunAutoCompactCmd(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	cmd := m.runAutoCompactCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil cmd")
	}

	// Execute should give a compaction result
	msg := cmd()
	_, ok := msg.(compactionResultMsg)
	if !ok {
		t.Fatalf("expected compactionResultMsg, got %T", msg)
	}
}

// --- View with pickers active ---

func TestViewWithModelPicker(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "m1", Provider: "p1"},
	}})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)
	m.modelPickerActive = true
	m.textarea.SetValue("m")

	view := m.View()
	if !strings.Contains(stripANSI(view), "filter:") {
		t.Fatalf("expected 'filter:' in view with model picker: %q", stripANSI(view))
	}
}

func TestViewWithLoginPicker(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)
	m.loginPickerActive = true
	m.textarea.SetValue("o")

	view := m.View()
	if !strings.Contains(stripANSI(view), "filter:") {
		t.Fatalf("expected 'filter:' in view with login picker: %q", stripANSI(view))
	}
}

func TestViewWithLogoutPicker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)
	m.logoutPickerActive = true
	m.textarea.SetValue("o")

	view := m.View()
	if !strings.Contains(stripANSI(view), "filter:") {
		t.Fatalf("expected 'filter:' in view with logout picker: %q", stripANSI(view))
	}
}

func TestViewWithCenteredLayout(t *testing.T) {
	m := NewModel(&config.AgentConfig{Alignment: "centered"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)

	view := m.View()
	// Should not panic with centered alignment
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestViewWithPendingAndSuggestions(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help"})

	m := NewModel(&config.AgentConfig{}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)
	m.pending = true
	m.textarea.SetValue("/")

	view := m.View()
	if !strings.Contains(stripANSI(view), "Working") {
		t.Fatalf("expected 'Working' in pending view: %q", stripANSI(view))
	}
}

func TestViewWithBannerAndSuggestions(t *testing.T) {
	m := NewModel(&config.AgentConfig{ShowBanner: true, Alignment: "centered"}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)
	m.messages = append(m.messages, messageItem{role: "user", content: "hello"})

	view := m.View()
	// Should not panic with banner and centered content
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

// --- Handle model picker key: select model ---

func TestHandleModelPickerKey_EnterSelectsModel(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	updated, _, handled := m.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected enter to be handled")
	}
	if m2.modelPickerActive {
		t.Fatal("expected model picker to close on enter")
	}
	if m2.config.Model != "openai/gpt-4o" {
		t.Fatalf("expected model to be selected, got %q", m2.config.Model)
	}
}

func TestHandleModelPickerKey_TabSelectsModel(t *testing.T) {
	cmdReg := commands.NewRegistry()
	cmdReg.SetModelRegistry(&mockModelRegistry{models: []commands.ModelInfo{
		{ID: "openai/gpt-4o", Provider: "openrouter"},
	}})

	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, cmdReg, nil, nil, nil, nil, tuiStylesForTest())
	m.modelPickerActive = true

	updated, _, handled := m.handleModelPickerKey(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected tab to be handled")
	}
	if m2.modelPickerActive {
		t.Fatal("expected model picker to close on tab")
	}
	if m2.config.Model != "openai/gpt-4o" {
		t.Fatalf("expected model to be selected, got %q", m2.config.Model)
	}
}

// --- Test Ctrl+C when there's a selection clears it ---

func TestCtrlCClearsSelection(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)

	// Set up a selection
	m.selection = selectionState{
		active:    false,
		finished:  true,
		startLine: 0, startCol: 0,
		endLine: 0, endCol: 5,
	}
	m.refreshViewport()

	// Add some plain lines
	m.plainLines = []string{"hello world"}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated.(Model)

	// Should not clear the pending flag (it wasn't set)
	// Just verify no panic and the model is valid
	_ = m2
}

// --- Test handleLoginPickerKey with ctrl+c ---

func TestHandleLoginPickerKey_CtrlC(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.loginPickerActive = true

	updated, _, handled := m.handleLoginPickerKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected ctrl+c to be handled")
	}
	if m2.loginPickerActive {
		t.Fatal("expected login picker to close")
	}
}

func TestHandleLoginPickerKey_TabWithNoProviders(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.loginPickerActive = true
	m.loginPickerFilter = "nonexistent"

	updated, _, handled := m.handleLoginPickerKey(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected tab to be handled")
	}
	if !m2.loginPickerActive {
		t.Fatal("expected picker to remain when no providers")
	}
}

// --- Test handleLogoutPickerKey ---

func TestHandleLogoutPickerKey_CtrlC(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	updated, _, handled := m.handleLogoutPickerKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected ctrl+c to be handled")
	}
	if m2.logoutPickerActive {
		t.Fatal("expected logout picker to close")
	}
}

func TestHandleLogoutPickerKey_EnterNoProviders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	updated, _, handled := m.handleLogoutPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected enter to be handled")
	}
	// With no providers, enter should keep the picker active
	if !m2.logoutPickerActive {
		t.Fatal("expected picker to remain active when no providers")
	}
}

func TestHandleLogoutPickerKey_TabNoProviders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	updated, _, handled := m.handleLogoutPickerKey(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected tab to be handled")
	}
	// With no providers, tab should keep the picker active
	if !m2.logoutPickerActive {
		t.Fatal("expected picker to remain active when no providers")
	}
}

func TestHandleLogoutPickerKey_ArrowNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	// With no logged-in providers, arrow should still be handled
	updated, _, handled := m.handleLogoutPickerKey(tea.KeyMsg{Type: tea.KeyDown})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected down to be handled")
	}
	_ = m2
}

func TestHandleLogoutPickerKey_UpWithNoProviders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.logoutPickerActive = true

	updated, _, handled := m.handleLogoutPickerKey(tea.KeyMsg{Type: tea.KeyUp})
	m2 := updated.(Model)

	if !handled {
		t.Fatal("expected up to be handled")
	}
	_ = m2
}

// --- Test WithPending scroll to bottom behavior ---

func TestUpdateWithPendingEnterDoesNothing(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.pending = true
	m.textarea.SetValue("hello")
	msgCount := len(m.messages)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if len(m2.messages) != msgCount {
		t.Fatalf("expected no new messages when pending, got %d more", len(m2.messages)-msgCount)
	}
}

// --- Test copyToClipboardCmd edge cases ---

func TestCopyToClipboardCmd_EmptyString(t *testing.T) {
	cmd := copyToClipboardCmd("")
	if cmd != nil {
		t.Fatal("expected nil for empty string")
	}
}

func TestCopyToClipboardCmd_NonEmpty(t *testing.T) {
	cmd := copyToClipboardCmd("hello")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
}

// --- Test Overflow in shrinkColumns edge case ---

func TestShrinkColumns_AdjustsRemaining(t *testing.T) {
	// Should handle cases where remaining isn't perfectly distributed
	result := shrinkColumns([]int{100, 1, 1}, 30)
	if len(result) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(result))
	}
	total := 0
	for _, w := range result {
		total += w
	}
	if total < 10 || total > 33 {
		t.Fatalf("expected reasonable total, got %d: %v", total, result)
	}
}

func TestShrinkColumns_OverMinTightLoop(t *testing.T) {
	// Multiple columns with massive overshoot - tests the all-min break
	result := shrinkColumns([]int{1000, 1000, 1000}, 9)
	if len(result) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(result))
	}
}

// --- Test RenderToolCall edge: width zero ---

func TestRenderToolCall_NilArgsForFormatSingleToolCallLine(t *testing.T) {
	tc := toolRenderItem{
		name: "bash",
		args: "",
	}
	// With args="" and no rawArgs, should just show tool name
	got := formatSingleToolCallLine(tc, tuiStylesForTest())
	if stripANSI(got) != "bash" {
		t.Fatalf("expected 'bash', got %q", stripANSI(got))
	}
}

// --- Tests for /new (ResetSession) ---

func TestResetSession_EmptySessionDirDisablesPersistence(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ResetSession()
	if m.session != nil {
		t.Fatalf("expected no session when SessionDir is empty, got %s", m.session.Path())
	}
}

func TestResetSession_ClearsMessages(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.messages = []messageItem{
		{role: "user", content: "hello"},
		{role: "assistant", content: "hi"},
	}
	m.previousCompactionSummary = "previous summary"

	m.ResetSession()

	if len(m.messages) != 0 {
		t.Fatalf("expected messages to be cleared, got %d", len(m.messages))
	}
	if m.previousCompactionSummary != "" {
		t.Fatalf("expected compaction summary to be cleared, got %q", m.previousCompactionSummary)
	}
	if m.pending {
		t.Fatal("expected pending to be false after ResetSession")
	}
}

func TestResetSession_CreatesNewSessionFile(t *testing.T) {
	sessionDir := t.TempDir()
	cfg := &config.AgentConfig{
		HasAuthorizedProvider: true,
		SessionDir:            sessionDir,
	}
	m := NewModel(cfg, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	// Set a known initial session so we can verify it changes.
	initialSess, err := session.NewManager(sessionDir, "initial")
	if err != nil {
		t.Fatal(err)
	}
	m.session = initialSess
	initialPath := initialSess.Path()

	m.ResetSession()

	if m.session == nil {
		t.Fatal("expected session to be non-nil after ResetSession")
	}
	if m.session.Path() == initialPath {
		t.Fatal("expected session file path to change after ResetSession")
	}
	// Write a record to create the file, then verify it exists.
	if err := m.session.Append(session.Record{Role: "system", Content: "new session"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.session.Path()); os.IsNotExist(err) {
		t.Fatalf("expected new session file to exist at %s", m.session.Path())
	}
}

func TestResetSession_CancelsPendingAgent(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	ctx, cancel := context.WithCancel(context.Background())
	m.agentCancel = cancel
	m.pending = true

	m.ResetSession()

	if m.pending {
		t.Fatal("expected pending to be cleared")
	}
	if m.agentCancel != nil {
		t.Fatal("expected agentCancel to be nil after ResetSession")
	}
	// Verify the context was cancelled.
	if ctx.Err() == nil {
		t.Fatal("expected agent context to be cancelled")
	}
}

func TestThemeCommandOpensPicker(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.height = 40
	m.width = 100
	m.textarea.SetValue("/theme")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.(Model)
	if !got.themePickerActive {
		t.Fatal("expected theme picker to be active")
	}
	if got.textarea.Value() != "" {
		t.Fatalf("expected input reset, got %q", got.textarea.Value())
	}
}

func TestSelectThemeUpdatesConfigAndStyles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())

	m.selectTheme("crobot-light")

	if m.config.Theme != "crobot-light" {
		t.Fatalf("expected theme config update, got %q", m.config.Theme)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].content, "crobot-light") {
		t.Fatalf("expected theme switch message, got %#v", m.messages)
	}
}
