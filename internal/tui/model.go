package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"crobot/internal/agent"
	"crobot/internal/commands"
	"crobot/internal/compaction"
	"crobot/internal/config"
	"crobot/internal/events"
	"crobot/internal/prompt"
	"crobot/internal/provider"
	"crobot/internal/session"
	"crobot/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// toolState tracks the lifecycle of a tool call in the UI.
type toolState int

const (
	toolPending toolState = iota // args received, not yet executing
	toolRunning                  // executing
	toolDone                     // finished (success or error)
)

// toolRenderItem holds rendered state for one tool call in the view.
type toolRenderItem struct {
	name     string
	callID   string
	args     string
	rawArgs  map[string]any
	output   string
	success  bool
	duration time.Duration
	state    toolState
}

// messageItem holds one rendered message in the conversation view.
type messageItem struct {
	role      string // "user", "assistant", "system", "error"
	content   string
	reasoning string
	usage     string
	toolCalls []toolRenderItem
	ephemeral bool // if true, not sent to the agent as conversation history
}

// tea messages from the agent goroutine.
type agentEventMsg agent.Event
type agentDoneMsg struct{}

type compactionResultMsg struct {
	err    error
	result *compaction.Result
}

type loginResultMsg struct {
	provider  string
	accountID string
	err       error
}

type logoutResultMsg struct {
	provider string
	err      error
}

// sessionWriter is the subset of session.Manager used by the TUI.
type sessionWriter interface {
	Append(rec session.Record) error
	Path() string
}

const noProviderWarning = "No provider added. Add credentials to ~/.crobot/auth.json or use /login."

// Model is the root Bubble Tea model for the agent TUI.
type Model struct {
	config   *config.AgentConfig
	provider provider.Provider
	toolReg  *tools.Registry
	session  sessionWriter
	events   *events.Logger
	cmdReg   *commands.Registry
	modelReg *provider.ModelRegistry

	// apiKeyFor returns the API key for a provider name, or "" if not authorized.
	apiKeyFor func(string) string

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	messages               []messageItem
	pending                bool
	ready                  bool
	width                  int
	height                 int
	commandSuggestionIndex int
	agentCancel            context.CancelFunc
	agentEvents            chan agent.Event

	// Tracks the previous compaction summary for iterative summarization.
	previousCompactionSummary string

	// Model picker modal state
	modelPickerActive bool
	modelPickerFilter string
	modelPickerIndex  int

	// Login picker modal state
	loginPickerActive bool
	loginPickerFilter string
	loginPickerIndex  int

	// Logout picker modal state
	logoutPickerActive bool
	logoutPickerFilter string
	logoutPickerIndex  int

	// Text selection state for mouse drag-select + copy.
	selection  selectionState
	plainLines []string // unstyled viewport content lines, 1:1 with styled lines

	// Global toggle for tool output expansion (ctrl+o).
	toolOutputExpanded bool
}

// NewModel creates a fully initialized TUI model.
func NewModel(
	cfg *config.AgentConfig,
	prov provider.Provider,
	toolReg *tools.Registry,
	ev *events.Logger,
	cmdReg *commands.Registry,
	modelReg *provider.ModelRegistry,
	sess sessionWriter,
	apiKeyFor func(string) string,
) *Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.Placeholder = ""
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true)

	s := NewLoaderSpinner()
	messages := []messageItem{}
	if prov == nil && !cfg.HasAuthorizedProvider {
		messages = append(messages, messageItem{role: "error", content: noProviderWarning})
	}

	return &Model{
		config:      cfg,
		provider:    prov,
		toolReg:     toolReg,
		events:      ev,
		cmdReg:      cmdReg,
		modelReg:    modelReg,
		session:     sess,
		apiKeyFor:   apiKeyFor,
		textarea:    ta,
		spinner:     s,
		messages:    messages,
		agentEvents: make(chan agent.Event, 64),
	}
}

// Init returns the initial commands.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		footerHeight := 5
		if m.pending {
			footerHeight = 6
		}
		vpHeight := msg.Height - footerHeight
		if vpHeight < 10 {
			vpHeight = 10
		}
		m.viewport = viewport.New(msg.Width, vpHeight)
		m.viewport.YPosition = 0
		m.viewport.SetContent(m.renderMessages())

		m.textarea.SetWidth(msg.Width - 4)
		return m, nil

	case logoutResultMsg:
		m.pending = false
		m.textarea.Focus()
		if msg.err != nil {
			m.messages = append(m.messages, messageItem{role: "error", content: msg.err.Error()})
		} else {
			if m.config.Provider == msg.provider || (msg.provider == "openai-codex" && m.config.Provider == "openai") {
				m.provider = nil
				m.config.Provider = ""
				m.config.Model = ""
				_ = config.SaveConfig(m.config)
			}
			m.modelReg = provider.NewModelRegistry()
			if m.cmdReg != nil {
				m.cmdReg.SetModelRegistry(m.modelReg)
			}
			auth, authErr := config.LoadAuth()
			m.config.HasAuthorizedProvider = authErr == nil && auth.HasAuthorizedProvider()
			if !m.config.HasAuthorizedProvider {
				m.provider = nil
				m.config.Provider = ""
				m.config.Model = ""
				_ = config.SaveConfig(m.config)
				m.messages = append(m.messages, messageItem{role: "system", content: fmt.Sprintf("Logged out of %s", msg.provider), ephemeral: true})
				m.messages = append(m.messages, messageItem{role: "error", content: noProviderWarning})
			} else if err := m.reloadAuthorizedProviders(); err != nil {
				m.messages = append(m.messages, messageItem{role: "error", content: fmt.Sprintf("Logged out of %s; model refresh warning: %v", msg.provider, err)})
			} else {
				m.messages = append(m.messages, messageItem{role: "system", content: fmt.Sprintf("Logged out of %s", msg.provider), ephemeral: true})
			}
		}
		m.refreshViewport()
		return m, nil

	case loginResultMsg:
		m.pending = false
		m.textarea.Focus()
		if msg.err != nil {
			m.messages = append(m.messages, messageItem{role: "error", content: msg.err.Error()})
		} else {
			m.config.HasAuthorizedProvider = true
			m.config.Provider = msg.provider
			_ = config.SaveConfig(m.config)
			modelLoadErr := m.reloadAuthorizedProviders()
			content := fmt.Sprintf("Logged in to %s", msg.provider)
			if msg.accountID != "" {
				content += fmt.Sprintf(" (%s)", msg.accountID)
			}
			if modelLoadErr != nil {
				content += fmt.Sprintf("; model refresh warning: %v", modelLoadErr)
			}
			m.messages = append(m.messages, messageItem{role: "system", content: content, ephemeral: true})
		}
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		// Picker modes: intercept navigation/action keys, let typing through.
		if m.logoutPickerActive {
			model, cmd, handled := m.handleLogoutPickerKey(msg)
			if handled {
				return model, cmd
			}
			m = model.(Model)
		}
		if m.loginPickerActive {
			model, cmd, handled := m.handleLoginPickerKey(msg)
			if handled {
				return model, cmd
			}
			m = model.(Model)
		}
		if m.modelPickerActive {
			model, cmd, handled := m.handleModelPickerKey(msg)
			if handled {
				return model, cmd
			}
			// Fall through to textarea update for character/backspace input.
			m = model.(Model)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			// If there's a selection, copy it.
			if m.selection.hasSelection() || m.selection.finished {
				text := m.selection.selectedText(m.plainLines)
				m.selection.clear()
				m.refreshViewport()
				if text != "" {
					m.messages = append(m.messages, messageItem{role: "system", content: "Copied to clipboard.", ephemeral: true})
					m.refreshViewport()
					return m, copyToClipboardCmd(text)
				}
				return m, nil
			}
			// Cancel pending agent if running.
			if m.pending && m.agentCancel != nil {
				m.agentCancel()
			}
			return m, nil

		case tea.KeyCtrlO:
			m.toolOutputExpanded = !m.toolOutputExpanded
			m.refreshViewport()
			return m, nil

		case tea.KeyEsc:
			if m.pending && m.agentCancel != nil {
				m.agentCancel()
				m.pending = false
				m.textarea.Focus()
				m.messages = append(m.messages, messageItem{role: "system", content: "Cancelled.", ephemeral: true})
				m.refreshViewport()
			}
			if m.selection.hasSelection() || m.selection.finished {
				m.selection.clear()
				m.refreshViewport()
			}
			return m, nil

		case tea.KeyUp:
			if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
				m.commandSuggestionIndex--
				if m.commandSuggestionIndex < 0 {
					m.commandSuggestionIndex = len(suggestions) - 1
				}
				return m, nil
			}

		case tea.KeyDown:
			if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
				m.commandSuggestionIndex++
				if m.commandSuggestionIndex >= len(suggestions) {
					m.commandSuggestionIndex = 0
				}
				return m, nil
			}

		case tea.KeyTab:
			if !m.pending {
				if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
					m.completeCommandSuggestion(suggestions)
					return m, nil
				}
				return m, nil
			}

		case tea.KeyShiftTab:
			if !m.pending {
				if err := m.cycleThinkingLevel(); err != nil {
					m.messages = append(m.messages, messageItem{role: "error", content: err.Error()})
					m.refreshViewport()
				}
				return m, nil
			}

		case tea.KeyEnter:
			if m.pending {
				return m, nil
			}

			// Backslash+Enter inserts a newline (pi-mono workaround for terminals
			// without Shift+Enter support).
			value := m.textarea.Value()
			if strings.HasSuffix(value, "\\") {
				m.textarea.SetValue(strings.TrimSuffix(value, "\\") + "\n")
				return m, nil
			}

			input := strings.TrimSpace(value)

			// Normal command suggestion handling
			if suggestions := m.commandSuggestions(); len(suggestions) > 0 && !m.commandInputExactlyMatchesSuggestion(suggestions) {
				m.completeCommandSuggestion(suggestions)
				return m, nil
			}

			if input == "" {
				return m, nil
			}

			// /compact: trigger context compaction
			if strings.HasPrefix(input, "/compact") {
				if !compaction.CanCompact(messagesToCompaction(m.messages)) {
					m.messages = append(m.messages, messageItem{role: "error", content: "Nothing to compact (session already compacted or empty)."})
					m.refreshViewport()
					m.textarea.Reset()
					m.textarea.Focus()
					return m, nil
				}
				instructions := strings.TrimSpace(strings.TrimPrefix(input, "/compact"))
				m.messages = append(m.messages, messageItem{role: "system", content: "Compacting context...", ephemeral: true})
				m.refreshViewport()
				m.textarea.Reset()
				m.textarea.Blur()
				return m, m.startCompaction(instructions)
			}

			// /model: open the model picker
			if input == "/model" {
				m.modelPickerActive = true
				m.modelPickerFilter = ""
				m.modelPickerIndex = 0
				m.textarea.Reset()
				m.textarea.Focus()
				return m, nil
			}

			// /login: open the OAuth provider picker
			if input == "/login" {
				m.loginPickerActive = true
				m.loginPickerFilter = ""
				m.loginPickerIndex = 0
				m.textarea.Reset()
				m.textarea.Focus()
				return m, nil
			}

			// /logout: open the logged-in OAuth provider picker
			if input == "/logout" {
				m.logoutPickerActive = true
				m.logoutPickerFilter = ""
				m.logoutPickerIndex = 0
				m.textarea.Reset()
				m.textarea.Focus()
				return m, nil
			}

			m.textarea.Reset()
			m.textarea.Blur()

			// Slash commands.
			if strings.HasPrefix(input, "/") {
				if m.isQuitCommand(input) {
					return m, tea.Quit
				}
				result, err := m.cmdReg.Execute(input)
				if err != nil {
					m.messages = append(m.messages, messageItem{role: "error", content: err.Error()})
				} else if result != "" {
					m.messages = append(m.messages, messageItem{role: "system", content: result, ephemeral: true})
				}
				m.refreshViewport()
				m.textarea.Focus()
				return m, nil
			}

			// Preprocessors.
			if strings.HasPrefix(input, "!") {
				input = expandShellShortcut(m.toolReg, input)
			}
			input = expandFileRefs(m.toolReg, input)

			if m.provider == nil {
				message := noProviderWarning
				if m.config.HasAuthorizedProvider {
					message = "No provider selected. Select a provider before sending a message."
				}
				m.messages = append(m.messages, messageItem{role: "error", content: message})
				m.refreshViewport()
				m.textarea.Focus()
				return m, nil
			}

			if m.selection.hasSelection() || m.selection.finished {
				m.selection.clear()
			}

			m.messages = append(m.messages, messageItem{role: "user", content: input})
			m.refreshViewport()

			if m.session != nil {
				_ = m.session.Append(session.Record{Role: "user", Content: input, Timestamp: time.Now()})
			}

			m.pending = true
			ctx, cancel := context.WithCancel(context.Background())
			m.agentCancel = cancel
			m.agentEvents = make(chan agent.Event, 64)
			go m.startAgent(ctx, input)

			return m, tea.Batch(m.spinner.Tick, m.waitForEvents())

			// textarea handles this natively via its own keybindings.
		}

		if m.shouldHandleViewportKey(msg) {
			if m.selection.hasSelection() || m.selection.finished {
				m.selection.clear()
				m.refreshViewport()
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.MouseMsg:
		if m.handleMouseSelection(msg) {
			m.refreshViewport()
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case agentEventMsg:
		return m.handleAgentEvent(agent.Event(msg))

	case agentDoneMsg:
		m.pending = false
		// Check for auto-compaction after agent finishes.
		if m.provider != nil && m.shouldAutoCompact() {
			return m, m.runAutoCompactCmd()
		}
		return m, nil

	case compactionResultMsg:
		return m.handleCompactionResult(msg)

	case spinner.TickMsg:
		if m.pending {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	var cmd tea.Cmd
	prevValue := m.textarea.Value()
	m.textarea, cmd = m.textarea.Update(msg)
	if m.modelPickerActive {
		prev := m.modelPickerFilter
		m.modelPickerFilter = m.textarea.Value()
		if m.modelPickerFilter != prev {
			m.modelPickerIndex = 0
		}
	}
	if m.loginPickerActive {
		prev := m.loginPickerFilter
		m.loginPickerFilter = m.textarea.Value()
		if m.loginPickerFilter != prev {
			m.loginPickerIndex = 0
		}
	}
	if m.logoutPickerActive {
		prev := m.logoutPickerFilter
		m.logoutPickerFilter = m.textarea.Value()
		if m.logoutPickerFilter != prev {
			m.logoutPickerIndex = 0
		}
	}
	// Clear selection when user starts typing.
	if m.textarea.Value() != prevValue && (m.selection.hasSelection() || m.selection.finished) {
		m.selection.clear()
		m.refreshViewport()
	}
	m.clampCommandSuggestionIndex()
	return m, cmd
}

// waitForEvents returns a tea.Cmd that reads from the agentEvents channel.
func (m Model) waitForEvents() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.agentEvents
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg(ev)
	}
}

// handleAgentEvent routes an agent event to update the UI state.
func (m Model) handleAgentEvent(ev agent.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "message_start":
		if ev.MessageStart != nil {
			if ev.MessageStart.Role == "assistant" {
				m.messages = append(m.messages, messageItem{role: "assistant"})
				m.refreshViewport()
			}
		}

	case "text_delta":
		if ev.TextDelta != "" {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				m.messages[len(m.messages)-1].content += ev.TextDelta
				m.refreshViewport()
			}
		}

	case "reasoning_delta":
		if ev.ReasoningDelta != "" {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				m.messages[len(m.messages)-1].reasoning += ev.ReasoningDelta
				m.refreshViewport()
			}
		}

	case "tool_call_start":
		// Handled by tool_call_end.

	case "tool_call_end":
		if ev.ToolCallEnd != nil {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				last.toolCalls = append(last.toolCalls, toolRenderItem{
					name:    ev.ToolCallEnd.Name,
					callID:  ev.ToolCallEnd.CallID,
					args:    formatToolCallLine(ev.ToolCallEnd.Name, ev.ToolCallEnd.Args),
					rawArgs: ev.ToolCallEnd.Args,
					state:   toolPending,
				})
				m.refreshViewport()
			}
		}

	case "tool_exec_start":
		if ev.ToolExecStart != nil {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				for i := len(last.toolCalls) - 1; i >= 0; i-- {
					if last.toolCalls[i].callID == ev.ToolExecStart.CallID {
						last.toolCalls[i].state = toolRunning
						break
					}
				}
				m.refreshViewport()
			}
		}

	case "tool_exec_result":
		if ev.ToolExecResult != nil {
			ter := ev.ToolExecResult
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				for i := len(last.toolCalls) - 1; i >= 0; i-- {
					if last.toolCalls[i].callID == ter.CallID {
						last.toolCalls[i].output = ter.Output
						last.toolCalls[i].success = ter.Success
						last.toolCalls[i].duration = time.Duration(ter.Duration) * time.Millisecond
						last.toolCalls[i].state = toolDone
						break
					}
				}
				m.refreshViewport()
			}
		}

	case "turn_end":
		m.refreshViewport()

	case "message_end":
		m.pending = false
		m.textarea.Focus()

		if ev.MessageEnd != nil {
			usage := ""
			if ev.MessageEnd.Usage != nil {
				usage = fmt.Sprintf("  %d in / %d out",
					ev.MessageEnd.Usage.InputTokens, ev.MessageEnd.Usage.OutputTokens)
			}
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				m.messages[len(m.messages)-1].usage = usage
			}

			if m.session != nil && ev.MessageEnd.Text != "" {
				_ = m.session.Append(session.Record{
					Role:      "assistant",
					Content:   ev.MessageEnd.Text,
					Timestamp: time.Now(),
				})
			}
		}
		m.refreshViewport()

	case "error":
		if ev.Error != nil {
			m.pending = false
			m.textarea.Focus()
			m.messages = append(m.messages, messageItem{role: "error", content: ev.Error.Error()})
			m.refreshViewport()
		}
	}

	// Keep reading events while pending.
	if m.pending {
		return m, m.waitForEvents()
	}
	return m, nil
}

// View renders the full terminal UI.
func (m Model) View() string {
	if !m.ready {
		return "Loading...\n"
	}
	var b strings.Builder

	viewport := m.viewport
	if m.height > 0 {
		viewport.Height = m.dynamicViewportHeight()
	}
	b.WriteString(viewport.View())
	b.WriteString("\n")

	if m.modelPickerActive {
		picker := m.renderModelPicker()
		filter := Dim.Render("filter: ") + m.textarea.Value() + InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else if m.loginPickerActive {
		picker := m.renderLoginPicker()
		filter := Dim.Render("filter: ") + m.textarea.Value() + InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else if m.logoutPickerActive {
		picker := m.renderLogoutPicker()
		filter := Dim.Render("filter: ") + m.textarea.Value() + InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else {
		if m.pending {
			spinnerLine := m.spinner.View() + " " + Dim.Render("Working")
			if m.config.Alignment == "centered" {
				spinnerLine = centerContent(spinnerLine, m.width)
			}
			b.WriteString(spinnerLine)
			b.WriteString("\n")
		}

		if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
			sug := m.renderCommandSuggestions(suggestions)
			if m.config.Alignment == "centered" {
				sug = centerContent(sug, m.width)
			}
			b.WriteString(sug)
			b.WriteString("\n")
		}

		statusLine := m.renderStatusLine()
		if m.config.Alignment == "centered" {
			statusLine = centerContent(statusLine, m.width)
		}
		b.WriteString(statusLine)
		b.WriteString("\n")
		input := "> " + m.renderInputView()
		if m.config.Alignment == "centered" {
			input = centerContent(input, m.width)
		}
		b.WriteString(input)
	}
	b.WriteString("\n")
	cwd := Dim.Render(compactCwd())
	if m.config.Alignment == "centered" {
		cwd = centerContent(cwd, m.width)
	}
	b.WriteString(cwd)

	return b.String()
}

func (m Model) renderInputView() string {
	value := m.textarea.Value()
	if m.pending {
		return value
	}
	return value + InputCursor.Render("█")
}

func (m Model) renderStatusLine() string {
	providerName := valueOrDefault(m.config.Provider, "unknown")
	modelName := valueOrDefault(m.config.Model, "unknown")
	thinking := valueOrDefault(m.config.Thinking, "none")
	alignment := valueOrDefault(m.config.Alignment, "left")
	return Dim.Render(fmt.Sprintf("provider: %s  model: %s  thinking: %s  alignment: %s  shift+tab: cycle thinking", providerName, modelName, thinking, alignment))
}

func (m *Model) cycleThinkingLevel() error {
	levels := []string{"none", "minimal", "low", "medium", "high", "xhigh"}
	current := m.config.Thinking
	for i, level := range levels {
		if level == current {
			m.config.Thinking = levels[(i+1)%len(levels)]
			return config.SaveConfig(m.config)
		}
	}
	m.config.Thinking = levels[0]
	return config.SaveConfig(m.config)
}

func (m Model) commandSuggestions() []commands.Command {
	if m.cmdReg == nil || m.pending {
		return nil
	}
	return m.cmdReg.Suggestions(m.textarea.Value())
}

func (m Model) isQuitCommand(input string) bool {
	cmd, _, ok := commands.Parse(input)
	return ok && (cmd == "quit" || cmd == "exit")
}

func (m *Model) clampCommandSuggestionIndex() {
	suggestions := m.commandSuggestions()
	if len(suggestions) == 0 {
		m.commandSuggestionIndex = 0
		return
	}
	if m.commandSuggestionIndex >= len(suggestions) {
		m.commandSuggestionIndex = len(suggestions) - 1
	}
	if m.commandSuggestionIndex < 0 {
		m.commandSuggestionIndex = 0
	}
}

func (m *Model) completeCommandSuggestion(suggestions []commands.Command) {
	m.clampCommandSuggestionIndex()
	if len(suggestions) == 0 {
		return
	}

	selected := suggestions[m.commandSuggestionIndex]

	// Check if this is a model suggestion
	if selected.ModelID != "" {
		m.selectModel(selected.ModelProvider, selected.ModelID)
		return
	}

	// Normal command completion
	m.textarea.SetValue("/" + selected.Name + " ")
	m.commandSuggestionIndex = 0
}

// handleModelPickerKey processes key events when model picker is active.
// Returns (model, cmd, handled). If handled is false, the event falls through
// to the textarea for character/backspace input.
func (m Model) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	models := m.cmdReg.FilterModels(m.modelPickerFilter)

	switch msg.Type {
	case tea.KeyCtrlC:
		m.modelPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyEsc:
		m.modelPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyEnter:
		if len(models) > 0 {
			m.clampModelPickerIndex(models)
			selected := models[m.modelPickerIndex]
			m.selectModel(selected.ModelProvider, selected.ModelID)
		}
		m.modelPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyUp:
		if len(models) > 0 {
			m.modelPickerIndex--
			m.clampModelPickerIndex(models)
		}
		return m, nil, true

	case tea.KeyDown:
		if len(models) > 0 {
			m.modelPickerIndex++
			m.clampModelPickerIndex(models)
		}
		return m, nil, true

	case tea.KeyTab:
		if len(models) > 0 {
			m.clampModelPickerIndex(models)
			selected := models[m.modelPickerIndex]
			m.selectModel(selected.ModelProvider, selected.ModelID)
		}
		m.modelPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true
	}

	// Not a special key — let textarea handle it (backspace, characters, etc.)
	return m, nil, false
}

func (m *Model) clampModelPickerIndex(models []commands.Command) {
	if len(models) == 0 {
		m.modelPickerIndex = 0
		return
	}
	if m.modelPickerIndex < 0 {
		m.modelPickerIndex = 0
	}
	if m.modelPickerIndex >= len(models) {
		m.modelPickerIndex = len(models) - 1
	}
}

// renderModelPicker renders the model picker modal.
func (m Model) renderModelPicker() string {
	models := m.cmdReg.FilterModels(m.modelPickerFilter)

	var b strings.Builder

	if len(models) == 0 {
		b.WriteString(Dim.Render("  No models match your filter"))
		if m.modelPickerFilter == "" {
			b.WriteString(Dim.Render(" (no models available)"))
		}
		b.WriteString("\n")
	} else {
		b.WriteString(Dim.Render(fmt.Sprintf("  models (%d)", len(models))))
		b.WriteString("\n")

		m.clampModelPickerIndex(models)
		start, end, _ := m.visibleModelPickerRange(models)

		if start > 0 {
			b.WriteString(Dim.Render(fmt.Sprintf("  +%d more above", start)))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			mdl := models[i]
			prefix := "  "
			style := Dim
			if i == m.modelPickerIndex {
				prefix = "> "
				style = UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, mdl.ModelID)
			if mdl.Args != "" {
				line += Dim.Render(fmt.Sprintf("  (%s)", mdl.Args))
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
		if end < len(models) {
			b.WriteString(Dim.Render(fmt.Sprintf("  +%d more below", len(models)-end)))
			b.WriteString("\n")
		}
	}

	b.WriteString(Dim.Render("  esc: cancel  enter: select  arrows: navigate  type: filter"))

	return b.String()
}

type loginProviderOption struct {
	ID          string
	Name        string
	Description string
}

func oauthProviderOptions() []loginProviderOption {
	return []loginProviderOption{{ID: "openai-codex", Name: "OpenAI Codex", Description: "ChatGPT Plus/Pro OAuth"}}
}

func (m Model) filteredLoginProviders() []loginProviderOption {
	filter := strings.ToLower(strings.TrimSpace(m.loginPickerFilter))
	providers := oauthProviderOptions()
	if filter == "" {
		return providers
	}
	out := make([]loginProviderOption, 0, len(providers))
	for _, p := range providers {
		if strings.Contains(strings.ToLower(p.ID), filter) || strings.Contains(strings.ToLower(p.Name), filter) || strings.Contains(strings.ToLower(p.Description), filter) {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) handleLoginPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	providers := m.filteredLoginProviders()
	switch msg.Type {
	case tea.KeyCtrlC:
		m.loginPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEsc:
		m.loginPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEnter, tea.KeyTab:
		if len(providers) == 0 {
			return m, nil, true
		}
		m.clampLoginPickerIndex(providers)
		selected := providers[m.loginPickerIndex]
		m.loginPickerActive = false
		m.textarea.Reset()
		m.textarea.Blur()
		m.pending = true
		m.messages = append(m.messages, messageItem{role: "system", content: fmt.Sprintf("Opening browser for %s OAuth login...", selected.Name), ephemeral: true})
		m.refreshViewport()
		return m, m.loginProviderCmd(selected.ID), true
	case tea.KeyUp:
		if len(providers) > 0 {
			m.loginPickerIndex--
			m.clampLoginPickerIndex(providers)
		}
		return m, nil, true
	case tea.KeyDown:
		if len(providers) > 0 {
			m.loginPickerIndex++
			m.clampLoginPickerIndex(providers)
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) reloadAuthorizedProviders() error {
	auth, err := config.LoadAuth()
	if err != nil {
		return err
	}
	m.provider = nil
	for _, providerName := range []string{"openrouter", "openai", "openai-codex", "deepseek", "anthropic"} {
		apiKey := auth.APIKey(providerName)
		if apiKey == "" {
			continue
		}
		prov, err := provider.Create(providerName, apiKey)
		if err != nil {
			return err
		}
		if providerName == m.config.Provider {
			m.provider = prov
		}
		if m.modelReg != nil {
			m.modelReg.AddProvider(prov)
		}
	}
	if m.provider == nil && m.config.Provider != "" {
		if apiKey := auth.APIKey(m.config.Provider); apiKey != "" {
			if prov, err := provider.Create(m.config.Provider, apiKey); err == nil {
				m.provider = prov
			}
		}
	}
	if m.modelReg == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return m.modelReg.LoadModels(ctx)
}

func (m Model) loggedInOAuthProviders() []loginProviderOption {
	auth, err := config.LoadAuth()
	if err != nil {
		return nil
	}
	known := map[string]loginProviderOption{
		"openai-codex": {ID: "openai-codex", Name: "OpenAI Codex", Description: "ChatGPT Plus/Pro OAuth"},
	}
	seen := map[string]bool{}
	filter := strings.ToLower(strings.TrimSpace(m.logoutPickerFilter))
	var out []loginProviderOption
	for _, id := range auth.OAuthProviders() {
		if seen[id] {
			continue
		}
		seen[id] = true
		p, ok := known[id]
		if !ok {
			p = loginProviderOption{ID: id, Name: id, Description: "OAuth"}
		}
		if filter == "" || strings.Contains(strings.ToLower(p.ID), filter) || strings.Contains(strings.ToLower(p.Name), filter) || strings.Contains(strings.ToLower(p.Description), filter) {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) handleLogoutPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	providers := m.loggedInOAuthProviders()
	switch msg.Type {
	case tea.KeyCtrlC:
		m.logoutPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEsc:
		m.logoutPickerActive = false
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEnter, tea.KeyTab:
		if len(providers) == 0 {
			return m, nil, true
		}
		m.clampLogoutPickerIndex(providers)
		selected := providers[m.logoutPickerIndex]
		m.logoutPickerActive = false
		m.textarea.Reset()
		m.textarea.Blur()
		m.pending = true
		return m, m.logoutProviderCmd(selected.ID), true
	case tea.KeyUp:
		if len(providers) > 0 {
			m.logoutPickerIndex--
			m.clampLogoutPickerIndex(providers)
		}
		return m, nil, true
	case tea.KeyDown:
		if len(providers) > 0 {
			m.logoutPickerIndex++
			m.clampLogoutPickerIndex(providers)
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) clampLogoutPickerIndex(providers []loginProviderOption) {
	if len(providers) == 0 {
		m.logoutPickerIndex = 0
		return
	}
	if m.logoutPickerIndex < 0 {
		m.logoutPickerIndex = 0
	}
	if m.logoutPickerIndex >= len(providers) {
		m.logoutPickerIndex = len(providers) - 1
	}
}

func (m Model) renderLogoutPicker() string {
	providers := m.loggedInOAuthProviders()
	var b strings.Builder
	if len(providers) == 0 {
		b.WriteString(Dim.Render("  No logged-in OAuth providers"))
		b.WriteString("\n")
	} else {
		b.WriteString(Dim.Render(fmt.Sprintf("  logged-in OAuth providers (%d)", len(providers))))
		b.WriteString("\n")
		m.clampLogoutPickerIndex(providers)
		for i, p := range providers {
			prefix := "  "
			style := Dim
			if i == m.logoutPickerIndex {
				prefix = "> "
				style = UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, p.Name)
			if p.Description != "" {
				line += Dim.Render(fmt.Sprintf("  (%s)", p.Description))
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
	}
	b.WriteString(Dim.Render("  esc: cancel  enter: logout  arrows: navigate  type: filter"))
	return b.String()
}

func (m Model) logoutProviderCmd(providerID string) tea.Cmd {
	return func() tea.Msg {
		return logoutResultMsg{provider: providerID, err: config.LogoutOAuthProvider(providerID)}
	}
}

func (m Model) loginProviderCmd(providerID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		switch providerID {
		case "openai-codex":
			accountID, err := config.LoginOpenAIOAuth(ctx)
			return loginResultMsg{provider: "openai-codex", accountID: accountID, err: err}
		default:
			return loginResultMsg{provider: providerID, err: fmt.Errorf("unsupported oauth provider: %s", providerID)}
		}
	}
}

func (m *Model) clampLoginPickerIndex(providers []loginProviderOption) {
	if len(providers) == 0 {
		m.loginPickerIndex = 0
		return
	}
	if m.loginPickerIndex < 0 {
		m.loginPickerIndex = 0
	}
	if m.loginPickerIndex >= len(providers) {
		m.loginPickerIndex = len(providers) - 1
	}
}

func (m Model) renderLoginPicker() string {
	providers := m.filteredLoginProviders()
	var b strings.Builder
	if len(providers) == 0 {
		b.WriteString(Dim.Render("  No OAuth providers match your filter"))
		b.WriteString("\n")
	} else {
		b.WriteString(Dim.Render(fmt.Sprintf("  OAuth providers (%d)", len(providers))))
		b.WriteString("\n")
		m.clampLoginPickerIndex(providers)
		for i, p := range providers {
			prefix := "  "
			style := Dim
			if i == m.loginPickerIndex {
				prefix = "> "
				style = UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, p.Name)
			if p.Description != "" {
				line += Dim.Render(fmt.Sprintf("  (%s)", p.Description))
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
	}
	b.WriteString(Dim.Render("  esc: cancel  enter: login  arrows: navigate  type: filter"))
	return b.String()
}

func (m Model) visibleModelPickerRange(models []commands.Command) (start, end, selected int) {
	const maxVisible = 12

	selected = m.modelPickerIndex
	if selected >= len(models) {
		selected = len(models) - 1
	}
	if selected < 0 {
		selected = 0
	}

	end = len(models)
	if len(models) > maxVisible {
		start = selected - maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + maxVisible
		if end > len(models) {
			end = len(models)
			start = end - maxVisible
		}
	}
	return start, end, selected
}

// selectModel sets the provider/model and clears the input.
func (m *Model) selectModel(providerName, modelID string) {
	if providerName == "" {
		providerName = m.config.Provider
	}
	if providerName == "" {
		providerName = "openrouter"
	}
	m.config.Provider = providerName
	m.config.Model = modelID
	_ = config.SaveConfig(m.config)

	// Create the provider for the selected model. If the provider changed,
	// discard the old client so requests do not go to a stale endpoint.
	if m.provider == nil || m.provider.Name() != m.config.Provider {
		m.provider = nil
		if m.apiKeyFor != nil {
			apiKey := m.apiKeyFor(m.config.Provider)
			if apiKey != "" {
				prov, err := provider.Create(m.config.Provider, apiKey)
				if err == nil {
					m.provider = prov
					m.config.HasAuthorizedProvider = true
				}
			}
		}
	}

	m.textarea.Reset()
	m.commandSuggestionIndex = 0
	m.messages = append(m.messages, messageItem{
		role:      "system",
		content:   fmt.Sprintf("Model changed to %s (%s)", modelID, m.config.Provider),
		ephemeral: true,
	})
	m.refreshViewport()
	m.textarea.Focus()
}

func (m Model) commandInputExactlyMatchesSuggestion(suggestions []commands.Command) bool {
	input := strings.TrimSpace(m.textarea.Value())
	if !strings.HasPrefix(input, "/") {
		return false
	}
	name := strings.TrimPrefix(input, "/")
	for _, cmd := range suggestions {
		if cmd.Name == name {
			return true
		}
	}
	return false
}

func (m Model) renderCommandSuggestions(suggestions []commands.Command) string {
	if len(suggestions) == 0 {
		return ""
	}

	start, end, selected := m.visibleCommandSuggestionRange(suggestions)

	var b strings.Builder

	// Check if these are model suggestions
	isModel := len(suggestions) > 0 && suggestions[0].ModelID != ""
	if isModel {
		b.WriteString(Dim.Render(fmt.Sprintf("models (%d)", len(suggestions))))
	} else {
		b.WriteString(Dim.Render("commands"))
	}

	if start > 0 {
		b.WriteString("\n")
		b.WriteString(Dim.Render(fmt.Sprintf("  +%d more above", start)))
	}
	for i := start; i < end; i++ {
		cmd := suggestions[i]
		prefix := "  "
		style := Dim
		if i == selected {
			prefix = "> "
			style = UserPrompt
		}

		var line string
		if isModel {
			line = fmt.Sprintf("%s%s", prefix, cmd.ModelID)
			if cmd.Args != "" {
				line += Dim.Render(fmt.Sprintf("  (%s)", cmd.Args))
			}
		} else {
			line = fmt.Sprintf("%s/%s", prefix, cmd.Name)
			if cmd.Args != "" {
				line += " " + cmd.Args
			}
			if cmd.Description != "" {
				line += "  " + cmd.Description
			}
		}

		b.WriteString("\n")
		b.WriteString(style.Render(line))
	}
	if end < len(suggestions) {
		b.WriteString("\n")
		b.WriteString(Dim.Render(fmt.Sprintf("  +%d more below", len(suggestions)-end)))
	}
	return b.String()
}

func (m Model) visibleCommandSuggestionRange(suggestions []commands.Command) (start, end, selected int) {
	const maxVisible = 8

	selected = m.commandSuggestionIndex
	if selected >= len(suggestions) {
		selected = len(suggestions) - 1
	}
	if selected < 0 {
		selected = 0
	}

	end = len(suggestions)
	if len(suggestions) > maxVisible {
		start = selected - maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + maxVisible
		if end > len(suggestions) {
			end = len(suggestions)
			start = end - maxVisible
		}
	}
	return start, end, selected
}

func (m Model) commandSuggestionHeight() int {
	suggestions := m.commandSuggestions()
	if len(suggestions) == 0 {
		return 0
	}
	start, end, _ := m.visibleCommandSuggestionRange(suggestions)
	height := 1 + end - start
	if start > 0 {
		height++
	}
	if end < len(suggestions) {
		height++
	}
	return height
}

func (m Model) modelPickerHeight() int {
	models := m.cmdReg.FilterModels(m.modelPickerFilter)
	if len(models) == 0 {
		return 2 // empty message + help line
	}
	start, end, _ := m.visibleModelPickerRange(models)
	height := 1 + end - start // header + visible models
	if start > 0 {
		height++
	}
	if end < len(models) {
		height++
	}
	return height + 1 // help line
}

func (m Model) dynamicViewportHeight() int {
	footerHeight := 5 + m.commandSuggestionHeight()
	if m.modelPickerActive {
		footerHeight = 3 + m.modelPickerHeight()
	}
	if m.pending {
		footerHeight++
	}
	vpHeight := m.height - footerHeight
	if vpHeight < 3 {
		vpHeight = 3
	}
	return vpHeight
}

func (m Model) renderMessages() string {
	var b strings.Builder
	wrapWidth := m.width
	if wrapWidth < 40 {
		wrapWidth = 40
	}
	if m.config.Alignment == "centered" {
		wrapWidth = wrapWidth * 3 / 4
		if wrapWidth < 40 {
			wrapWidth = 40
		}
	}
	if m.config.ShowBanner {
		b.WriteString(Render(m.config.Model))
		b.WriteString("\n")
	}
	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString("  ")
			b.WriteString(UserCaret.Render(">"))
			b.WriteString(" ")
			b.WriteString(UserPrompt.Render(msg.content))
			b.WriteString("\n\n")
		case "assistant":
			if msg.reasoning != "" && m.config.Reasoning {
				b.WriteString(ThinkingStyle.Render("thinking"))
				b.WriteString("\n")
				b.WriteString(ThinkingStyle.Render(wrapText(msg.reasoning, wrapWidth)))
				b.WriteString("\n")
			}
			if msg.content != "" {
				b.WriteString(RenderMarkdown(msg.content, wrapWidth))
			}
			for _, tc := range msg.toolCalls {
				tcWidth := m.width - 4
				if m.config.Alignment == "centered" {
					tcWidth = m.width * 3 / 4
					if tcWidth < 40 {
						tcWidth = 40
					}
				}
				b.WriteString(RenderToolCall(tc, tcWidth, m.toolOutputExpanded))
				b.WriteString("\n")
			}
			if msg.usage != "" {
				b.WriteString(Gray.Render(msg.usage))
				b.WriteString("\n")
			}
		case "system":
			b.WriteString(Dim.Render(wrapText(msg.content, wrapWidth)))
			b.WriteString("\n\n")
		case "compaction":
			b.WriteString(Dim.Render("[compaction] "))
			b.WriteString(Dim.Render(wrapText(msg.content, wrapWidth)))
			b.WriteString("\n\n")
		case "error":
			errWidth := wrapWidth - 7
			if errWidth < 20 {
				errWidth = 20
			}
			b.WriteString(Red.Render("Error: " + wrapText(msg.content, errWidth)))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func (m Model) shouldHandleViewportKey(msg tea.KeyMsg) bool {
	if len(m.commandSuggestions()) > 0 {
		return false
	}

	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyCtrlU, tea.KeyCtrlD:
		return true
	case tea.KeyUp, tea.KeyDown:
		return strings.TrimSpace(m.textarea.Value()) == ""
	default:
		return false
	}
}

// handleMouseSelection processes mouse events for text selection in the viewport.
// Returns true if the event was consumed (selection-related).
func (m *Model) handleMouseSelection(msg tea.MouseMsg) bool {
	// Only handle in the viewport area (Y < viewport height).
	// Use dynamic height since View() applies it at render time.
	vpHeight := m.dynamicViewportHeight()
	if msg.Y >= vpHeight {
		// Click outside viewport — clear selection if any.
		if m.selection.hasSelection() || m.selection.finished {
			m.selection.clear()
			return true
		}
		return false
	}

	// Map screen Y to content line index.
	contentLine := m.viewport.YOffset + msg.Y

	// Get the styled line to map screen X to plain text column.
	// Use a viewport copy with the dynamic height, since View() uses Height internally.
	// Note: vp.View() may return fewer lines than vpHeight if content is shorter.
	vp := m.viewport
	vp.Height = vpHeight
	styledLines := strings.Split(vp.View(), "\n")
	if msg.Y < 0 {
		return false
	}

	var styledLine string
	if msg.Y < len(styledLines) {
		styledLine = styledLines[msg.Y]
	}
	plainCol := styledColToPlainOffset(styledLine, msg.X)

	switch {
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		// Start selection.
		m.selection.active = true
		m.selection.finished = false
		m.selection.startLine = contentLine
		m.selection.startCol = plainCol
		m.selection.endLine = contentLine
		m.selection.endCol = plainCol
		return true

	case msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft:
		// Update selection while dragging.
		if !m.selection.active {
			return false
		}
		m.selection.endLine = contentLine
		m.selection.endCol = plainCol
		return true

	case msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft:
		// Finish selection.
		if !m.selection.active {
			return false
		}
		m.selection.active = false
		m.selection.finished = true
		m.selection.endLine = contentLine
		m.selection.endCol = plainCol
		return true

	case msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown:
		// Allow viewport scrolling — clear selection on scroll.
		if m.selection.hasSelection() || m.selection.finished {
			m.selection.clear()
			m.refreshViewport()
		}
		return false
	}

	return false
}

func (m *Model) refreshViewport() {
	wasAtBottom := m.viewport.AtBottom()
	content := m.renderViewportContent()
	m.viewport.SetContent(content)
	m.plainLines = strings.Split(stripANSI(content), "\n")
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// renderViewportContent returns the full viewport content, with selection overlay if active.
func (m Model) renderViewportContent() string {
	content := m.renderMessages()
	if m.config.Alignment == "centered" {
		content = centerContent(content, m.width)
	}
	if m.selection.hasSelection() || m.selection.finished {
		content = overlaySelection(content, m.selection)
	}
	return content
}

// centerContent center-aligns each line within the given width.
func centerContent(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(line)
	}
	return strings.Join(lines, "\n")
}

// --- Agent runner goroutine ---

// messageToCompactionItem converts a TUI messageItem to a compaction.MessageItem.
func messageToCompactionItem(msg messageItem) compaction.MessageItem {
	result := compaction.MessageItem{
		Role:      msg.role,
		Content:   msg.content,
		Reasoning: msg.reasoning,
		ToolCalls: make([]compaction.ToolRenderItem, len(msg.toolCalls)),
	}
	for i, tc := range msg.toolCalls {
		result.ToolCalls[i] = compaction.ToolRenderItem{
			Name:    tc.name,
			CallID:  tc.callID,
			Output:  tc.output,
			Args:    tc.args,
			RawArgs: tc.rawArgs,
		}
	}
	return result
}

// compactionToMessageItem converts a compaction.MessageItem to a TUI messageItem.
func compactionToMessageItem(msg compaction.MessageItem) messageItem {
	result := messageItem{
		role:      msg.Role,
		content:   msg.Content,
		reasoning: msg.Reasoning,
		toolCalls: make([]toolRenderItem, len(msg.ToolCalls)),
	}
	for i, tc := range msg.ToolCalls {
		result.toolCalls[i] = toolRenderItem{
			name:    tc.Name,
			callID:  tc.CallID,
			output:  tc.Output,
			args:    tc.Args,
			rawArgs: tc.RawArgs,
		}
	}
	return result
}

// startCompaction launches compaction in a goroutine and returns a tea.Cmd that
// waits for the result via a channel. This mirrors the agent pattern (chan-based)
// and ensures Bubble Tea's event loop stays engaged during the LLM call.
func (m *Model) startCompaction(instructions string) tea.Cmd {
	ch := make(chan compactionResultMsg, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- compactionResultMsg{err: fmt.Errorf("compaction panic: %v", r)}
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if m.provider == nil {
			ch <- compactionResultMsg{err: fmt.Errorf("no provider configured")}
			return
		}

		compactionMsgs := messagesToCompaction(m.messages)
		result, err := compaction.Compact(ctx, m.provider, m.config.Model, m.config.Compaction, compactionMsgs, instructions, m.previousCompactionSummary)
		if err != nil {
			ch <- compactionResultMsg{err: err}
			return
		}
		ch <- compactionResultMsg{result: result}
	}()
	return func() tea.Msg {
		return <-ch
	}
}

// handleCompactionResult applies the compaction result to the model state.
func (m *Model) handleCompactionResult(msg compactionResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.messages = append(m.messages, messageItem{role: "error", content: fmt.Sprintf("Compaction failed: %v", msg.err)})
		m.refreshViewport()
		m.textarea.Reset()
		m.textarea.Focus()
		return m, nil
	}

	result := msg.result
	// Store summary for iterative compaction.
	m.previousCompactionSummary = result.Summary

	// Replace messages with compacted version.
	m.messages = make([]messageItem, len(result.NewMessages))
	for i, mi := range result.NewMessages {
		m.messages[i] = compactionToMessageItem(mi)
	}

	status := fmt.Sprintf("Context compacted — %d tokens summarized.", result.TokensBefore)
	m.messages = append(m.messages, messageItem{role: "system", content: status, ephemeral: true})
	m.refreshViewport()
	m.textarea.Reset()
	m.textarea.Focus()
	return m, nil
}

// shouldAutoCompact checks whether auto-compaction should trigger.
func (m *Model) shouldAutoCompact() bool {
	return compaction.ShouldCompact(messagesToCompaction(m.messages), m.config.Compaction)
}

// runAutoCompactCmd returns a tea.Cmd that runs auto-compaction in the background.
func (m *Model) runAutoCompactCmd() tea.Cmd {
	return m.startCompaction("")
}

// messagesToCompaction converts the TUI messageItem slice to compaction.MessageItem slice.
func messagesToCompaction(msgs []messageItem) []compaction.MessageItem {
	result := make([]compaction.MessageItem, len(msgs))
	for i, msg := range msgs {
		result[i] = messageToCompactionItem(msg)
	}
	return result
}

func (m *Model) startAgent(ctx context.Context, input string) {
	// Capture the channel locally so this goroutine only ever touches
	// its own instance, even if m.agentEvents is reassigned later.
	ch := m.agentEvents
	defer close(ch)

	sysPrompt := prompt.Build(*m.config, getCwd())

	// Build conversation history for the LLM, preserving assistant tool calls
	// immediately followed by their matching tool results.
	var llmMsgs []provider.Message
	for _, msg := range m.messages {
		// Skip ephemeral messages — they are UI-only notifications
		// (model changes, login/logout, cancellation, compaction status, etc.)
		// and should not pollute the agent's conversation history.
		if msg.role != "assistant" && msg.role != "user" && msg.ephemeral {
			continue
		}
		switch msg.role {
		case "user", "system", "compaction":
			role := msg.role
			if role == "compaction" {
				role = "system"
			}
			llmMsgs = append(llmMsgs, provider.Message{Role: role, Content: msg.content})
		case "assistant":
			llmMsg := provider.Message{Role: "assistant", Content: msg.content, ReasoningContent: msg.reasoning}
			for _, tc := range msg.toolCalls {
				if tc.callID != "" {
					llmMsg.ToolCalls = append(llmMsg.ToolCalls, provider.ToolCall{
						Name: tc.name,
						ID:   tc.callID,
						Args: tc.rawArgs,
					})
				}
			}
			llmMsgs = append(llmMsgs, llmMsg)
			for _, tc := range msg.toolCalls {
				if tc.output != "" {
					llmMsgs = append(llmMsgs, provider.Message{
						Role:       "tool",
						ToolCallID: tc.callID,
						Content:    tc.output,
					})
				}
			}
		}
	}

	// The latest user message is already in m.messages.
	// Run the agent loop.
	_, _ = agent.RunWithThinking(
		ctx,
		m.provider,
		m.config.Model,
		m.config.Thinking,
		m.config.MaxTurns,
		sysPrompt,
		llmMsgs,
		m.toolReg,
		nil, // plugins
		func(ev agent.Event) {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		},
	)
}

// --- Input preprocessors ---

func expandShellShortcut(reg *tools.Registry, input string) string {
	cmd := input[1:]
	result, err := reg.Execute(context.Background(), "bash", map[string]any{"command": cmd})
	if err != nil {
		return fmt.Sprintf("bash error: %v", err)
	}
	if r, ok := result.(map[string]any); ok {
		stdout, _ := r["stdout"].(string)
		stderr, _ := r["stderr"].(string)
		code, _ := r["exitCode"].(int)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("--- Output of: %s (exit %d) ---\n", cmd, code))
		if stdout != "" {
			b.WriteString(stdout)
		}
		if stderr != "" {
			b.WriteString(stderr)
		}
		b.WriteString("---")
		return b.String()
	}
	return fmt.Sprintf("%v", result)
}

func expandFileRefs(reg *tools.Registry, input string) string {
	parts := strings.Split(input, " ")
	for i, part := range parts {
		if strings.HasPrefix(part, "@") {
			path := part[1:]
			result, err := reg.Execute(context.Background(), "file_read", map[string]any{"path": path})
			if err != nil {
				continue
			}
			if r, ok := result.(map[string]any); ok {
				content, _ := r["content"].(string)
				if content != "" {
					parts[i] = fmt.Sprintf("\n--- Content of %s ---\n%s\n---", path, content)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

// --- Input style renderers ---

// --- Helpers ---

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func compactCwd() string {
	cwd := getCwd()
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	return cwd
}

func getCwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

func truncateDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

// formatToolCallLine produces a concise natural-language description of a tool
// call, following the pi-mono pattern: bold tool name followed by context-specific
// formatting of the most salient argument.
func formatToolCallLine(name string, args map[string]any) string {
	if args == nil {
		return name
	}
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			return "$ " + cmd
		}
		return name
	case "file_read", "read":
		return formatFilePathCall(name, args, "path", "offset", "limit")
	case "file_write", "write":
		return formatFilePathCall(name, args, "path", "", "")
	case "file_edit", "edit":
		return formatFilePathCall(name, args, "path", "", "")
	case "grep":
		path, _ := args["path"].(string)
		pattern, _ := args["pattern"].(string)
		var b strings.Builder
		b.WriteString(name)
		if pattern != "" {
			b.WriteString(" /")
			b.WriteString(pattern)
			b.WriteString("/")
		}
		if path != "" && path != "." {
			b.WriteString(" in ")
			b.WriteString(shortenDisplayPath(path))
		}
		return b.String()
	case "find":
		path, _ := args["path"].(string)
		glob, _ := args["glob"].(string)
		var b strings.Builder
		b.WriteString(name)
		if glob != "" {
			b.WriteString(" ")
			b.WriteString(glob)
		}
		if path != "" && path != "." {
			b.WriteString(" in ")
			b.WriteString(shortenDisplayPath(path))
		}
		return b.String()
	case "ls":
		path, _ := args["path"].(string)
		if path != "" && path != "." {
			return name + " " + shortenDisplayPath(path)
		}
		return name
	default:
		key := summarizeKey(name)
		if v, ok := args[key]; ok {
			val := fmt.Sprintf("%v", v)
			if len(val) > 60 {
				val = val[:60] + "..."
			}
			return name + " " + val
		}
		return name
	}
}

// formatFilePathCall formats a file read/write/edit call line:
//   read path	o\file.go:1-20
//   write path	o\file.go
func formatFilePathCall(name string, args map[string]any, pathKey, offsetKey, limitKey string) string {
	path, _ := args[pathKey].(string)
	if path == "" {
		return name
	}
	short := shortenDisplayPath(path)

	// Check for alternate key names.
	offset := getIntArg(args, offsetKey, "offset", "start_line")
	limit := getIntArg(args, limitKey, "limit", "end_line")

	if offset > 0 || limit > 0 {
		start := offset
		if start <= 0 {
			start = 1
		}
		if limit > 0 {
			return fmt.Sprintf("%s %s:%d-%d", name, short, start, start+limit-1)
		}
		return fmt.Sprintf("%s %s:%d", name, short, start)
	}
	return name + " " + short
}

// getIntArg tries multiple key names for an integer argument.
func getIntArg(args map[string]any, keys ...string) int {
	for _, k := range keys {
		if k == "" {
			continue
		}
		switch v := args[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		}
	}
	return 0
}

// shortenDisplayPath shortens a file path for display by trimming the cwd prefix.
func shortenDisplayPath(p string) string {
	cwd := getCwd()
	if strings.HasPrefix(p, cwd+"/") {
		return p[len(cwd)+1:]
	}
	if strings.HasPrefix(p, cwd) && p != cwd {
		return p[len(cwd):]
	}
	return p
}

// wrapText word-wraps text to fit within the given width. It preserves existing
// newlines and breaks long lines at word boundaries. Lines with no spaces that
// exceed width are force-broken at the width boundary.
func wrapText(text string, width int) string {
	if width <= 0 || text == "" {
		return text
	}
	var result strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(wrapLine(line, width))
	}
	return result.String()
}

// wrapLine wraps a single line (no embedded newlines) to the given width.
// Handles ANSI escape sequences without breaking them.
func wrapLine(line string, width int) string {
	if ansiLen(line) <= width {
		return line
	}
	var b strings.Builder
	pos := 0
	visPos := 0
	lineStart := 0
	lastSpace := -1

	for pos < len(line) {
		// Skip ANSI escape sequence.
		if line[pos] == 0x1b {
			pos++
			if pos < len(line) && line[pos] == '[' {
				for pos++; pos < len(line); pos++ {
					if line[pos] >= '@' && line[pos] <= '~' {
						pos++
						break
					}
				}
			} else if pos < len(line) && line[pos] == ']' {
				for pos++; pos < len(line); pos++ {
					if line[pos] == 0x07 || (line[pos] == 0x1b && pos+1 < len(line) && line[pos+1] == '\\') {
						pos += 2
						break
					}
				}
			} else {
				pos++
			}
			continue
		}

		// Advance one rune.
		r, size := decodeRune(line, pos)
		if r == ' ' {
			lastSpace = pos
		}
		visPos++

		if visPos > width {
			if lastSpace > lineStart {
				b.WriteString(line[lineStart:lastSpace])
				b.WriteByte('\n')
				pos = lastSpace + 1
				lineStart = pos
				visPos = 0
				lastSpace = -1
			} else {
				// Force break at rune boundary (before the character that overflowed).
				b.WriteString(line[lineStart:pos])
				b.WriteByte('\n')
				lineStart = pos
				visPos = 0
			}
			continue
		}
		pos += size
	}
	// Remainder.
	if lineStart < len(line) {
		b.WriteString(strings.TrimLeft(line[lineStart:], " "))
	}
	return b.String()
}

// decodeRune decodes a single UTF-8 rune from s at byte position pos,
// returning the rune and its byte size.
func decodeRune(s string, pos int) (rune, int) {
	if pos >= len(s) {
		return 0, 0
	}
	b := s[pos]
	if b < 0x80 {
		return rune(b), 1
	}
	if b < 0xC0 {
		return rune(b), 1 // continuation byte — treat as single byte
	}
	// Multi-byte sequence.
	if b < 0xE0 {
		if pos+2 > len(s) {
			return rune(b), 1
		}
		return rune(b)&0x1F<<6 | rune(s[pos+1])&0x3F, 2
	}
	if b < 0xF0 {
		if pos+3 > len(s) {
			return rune(b), 1
		}
		return rune(b)&0x0F<<12 | rune(s[pos+1])&0x3F<<6 | rune(s[pos+2])&0x3F, 3
	}
	if pos+4 > len(s) {
		return rune(b), 1
	}
	return rune(b)&0x07<<18 | rune(s[pos+1])&0x3F<<12 | rune(s[pos+2])&0x3F<<6 | rune(s[pos+3])&0x3F, 4
}

// ansiLen returns the visible character count of a string, skipping ANSI sequences.
func ansiLen(s string) int {
	n := 0
	inANSI := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			inANSI = true
			continue
		}
		if inANSI {
			if s[i] >= '@' && s[i] <= '~' {
				if s[i] == '[' {
					continue
				}
				inANSI = false
			}
			continue
		}
		n++
	}
	return n
}

func summarizeKey(name string) string {
	switch name {
	case "bash":
		return "command"
	case "file_read", "file_write", "file_edit":
		return "path"
	default:
		return ""
	}
}
