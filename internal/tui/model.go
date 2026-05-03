package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	"crobot/internal/agent"
	"crobot/internal/commands"
	"crobot/internal/compaction"
	"crobot/internal/config"
	"crobot/internal/conversation"
	"crobot/internal/events"
	"crobot/internal/prompt"
	"crobot/internal/provider"
	"crobot/internal/runtime"
	"crobot/internal/session"
	"crobot/internal/skills"
	"crobot/internal/themes"
	"crobot/internal/tools"
	"crobot/internal/version"

	"github.com/charmbracelet/bubbles/cursor"
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
	usageData *provider.Usage
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
	Load() ([]session.Record, error)
	Path() string
	ID() string
	Info() (session.SessionInfo, error)
	SetTitleFromPrompt(prompt string) error
	ExportMarkdown(path string) error
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
	plugins  agent.PluginManager
	styles   Styles

	// modelHistory tracks recently used models for display priority.
	modelHistory *commands.ModelHistory

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

	// Theme picker modal state
	themePickerActive bool
	themePickerFilter string
	themePickerIndex  int

	// Login picker modal state
	loginPickerActive bool
	loginPickerFilter string
	loginPickerIndex  int

	// Logout picker modal state
	logoutPickerActive bool
	logoutPickerFilter string
	logoutPickerIndex  int

	// Resume picker modal state
	resumePickerActive bool
	resumePickerFilter string
	resumePickerIndex  int

	// Loaded skills (metadata only).
	skills []skills.Skill

	// Text selection state for mouse drag-select + copy.
	selection  selectionState
	plainLines []string // unstyled viewport content lines, 1:1 with styled lines

	// textareaCursorRune tracks the cursor position as a rune offset into the
	// textarea value. This is needed because the textarea's cursor row/col
	// are private fields, and we need the position to render the cursor at
	// the correct visual location when the user navigates with arrow keys.
	textareaCursorRune int

	// pasteStore holds full text of large pastes that were replaced with markers.
	// On submit, markers are expanded back to the original text.
	pasteStore   map[int]string
	pasteCounter int

	// Global toggle for tool output expansion (ctrl+o).
	toolOutputExpanded bool

	// messageHistory stores previously submitted non-empty inputs (newest at end)
	// for Up/Down arrow navigation, similar to a terminal's command history.
	messageHistory      []string
	messageHistoryPos   int    // -1 = not browsing, 0 = most recent, 1 = older, etc.
	messageHistorySaved string // saved current input when starting to browse
}

// NewModel creates a fully initialized TUI model.
func NewModel(
	cfg *config.AgentConfig,
	prov provider.Provider,
	toolReg *tools.Registry,
	ev *events.Logger,
	cmdReg *commands.Registry,
	modelReg *provider.ModelRegistry,
	modelHistory *commands.ModelHistory,
	sess sessionWriter,
	apiKeyFor func(string) string,
	skls []skills.Skill,
	s Styles,
	pluginOpt ...agent.PluginManager,
) *Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.Placeholder = ""
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	// Disable blinking cursor
	ta.Cursor.SetMode(cursor.CursorStatic)
	// Bubble Tea v1 parses LF (\n) as ctrl+j. Several terminals emit LF for Shift+Enter.
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j")
	ta.KeyMap.InsertNewline.SetEnabled(true)

	sp := NewLoaderSpinner()
	SetLoaderSpinnerStyle(&sp, s)
	messages := []messageItem{}
	var plugins agent.PluginManager
	if len(pluginOpt) > 0 {
		plugins = pluginOpt[0]
	}
	if sess != nil {
		if records, err := sess.Load(); err == nil {
			for _, rec := range records {
				if rec.Role == "user" || rec.Role == "assistant" {
					messages = append(messages, messageItem{role: rec.Role, content: rec.Content, reasoning: rec.Reasoning})
				}
			}
		}
	}
	if prov == nil && !cfg.HasAuthorizedProvider {
		messages = append(messages, messageItem{role: "error", content: noProviderWarning, ephemeral: true})
	}

	return &Model{
		config:       cfg,
		provider:     prov,
		toolReg:      toolReg,
		events:       ev,
		cmdReg:       cmdReg,
		modelReg:     modelReg,
		modelHistory: modelHistory,
		plugins:      plugins,
		session:      sess,
		apiKeyFor:    apiKeyFor,
		skills:       skls,
		textarea:     ta,
		spinner:      sp,
		styles:       s,
		messages:     messages,
		pasteStore:   make(map[int]string),
		agentEvents:  make(chan agent.Event, 64),
	}
}

// Init returns the initial commands.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
	)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.shouldInsertInputNewline(msg) {
		return m.insertInputNewline()
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		m.viewport = viewport.New(msg.Width, m.dynamicViewportHeight())
		m.viewport.YPosition = 0
		m.viewport.SetContent(m.renderViewportContent())
		m.viewport.GotoBottom()

		tw := m.textareaWidth()
		m.textarea.SetWidth(tw)
		m.textarea.SetHeight(m.inputVisualLineCount())
		return m, nil

	case logoutResultMsg:
		m.pending = false
		m.textarea.Focus()
		if msg.err != nil {
			m.messages = append(m.messages, messageItem{role: "error", content: msg.err.Error(), ephemeral: true})
		} else {
			if m.config.Provider == msg.provider || (msg.provider == "openai-codex" && m.config.Provider == "openai") {
				m.provider = nil
				m.config.Provider = ""
				m.config.Model = ""
				_ = config.ClearProviderModel()
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
				_ = config.ClearProviderModel()
				m.messages = append(m.messages, messageItem{role: "system", content: fmt.Sprintf("Logged out of %s", msg.provider), ephemeral: true})
				m.messages = append(m.messages, messageItem{role: "error", content: noProviderWarning, ephemeral: true})
			} else if err := m.reloadAuthorizedProviders(); err != nil {
				m.messages = append(m.messages, messageItem{role: "error", content: fmt.Sprintf("Logged out of %s; model refresh warning: %v", msg.provider, err), ephemeral: true})
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
			m.messages = append(m.messages, messageItem{role: "error", content: msg.err.Error(), ephemeral: true})
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
		if m.themePickerActive {
			model, cmd, handled := m.handleThemePickerKey(msg)
			if handled {
				return model, cmd
			}
			// Fall through to textarea update for character/backspace input.
			m = model.(Model)
		}
		if m.resumePickerActive {
			model, cmd, handled := m.handleResumePickerKey(msg)
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

			// Navigate message history backward (previous messages).
			if !m.pending && len(m.messageHistory) > 0 {
				if m.messageHistoryPos == -1 {
					m.messageHistorySaved = m.textarea.Value()
				}
				m.messageHistoryPos++
				if m.messageHistoryPos >= len(m.messageHistory) {
					m.messageHistoryPos = len(m.messageHistory) - 1
				}
				idx := len(m.messageHistory) - 1 - m.messageHistoryPos
				m.textarea.SetValue(m.messageHistory[idx])
				m.textareaCursorRune = len([]rune(m.messageHistory[idx]))
				m.textarea.CursorEnd()
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

			// Navigate message history forward (more recent messages).
			if !m.pending && m.messageHistoryPos >= 0 {
				if m.messageHistoryPos > 0 {
					m.messageHistoryPos--
					idx := len(m.messageHistory) - 1 - m.messageHistoryPos
					m.textarea.SetValue(m.messageHistory[idx])
					m.textareaCursorRune = len([]rune(m.messageHistory[idx]))
					m.textarea.CursorEnd()
					return m, nil
				}
				// Was at most recent (pos == 0), restore saved input.
				m.messageHistoryPos = -1
				m.textarea.SetValue(m.messageHistorySaved)
				m.textareaCursorRune = len([]rune(m.messageHistorySaved))
				m.textarea.CursorEnd()
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
					m.messages = append(m.messages, messageItem{role: "error", content: err.Error(), ephemeral: true})
					m.refreshViewport()
				}
				return m, nil
			}

		case tea.KeyEnter:
			if m.pending {
				return m, nil
			}

			input := strings.TrimSpace(m.textarea.Value())

			// Expand paste markers before processing the input.
			input = m.expandPasteMarkers(input)

			// Normal command suggestion handling
			if suggestions := m.commandSuggestions(); len(suggestions) > 0 && !m.commandInputExactlyMatchesSuggestion(suggestions) {
				m.completeCommandSuggestion(suggestions)
				return m, nil
			}

			if input == "" {
				return m, nil
			}

			// Save to message history for Up/Down navigation.
			m.saveToHistory(input)

			// /compact: trigger context compaction
			if strings.HasPrefix(input, "/compact") {
				if !compaction.CanCompact(messagesToCompaction(m.messages)) {
					m.messages = append(m.messages, messageItem{role: "error", content: "Nothing to compact (session already compacted or empty).", ephemeral: true})
					m.refreshViewport()
					m.resetTextarea()
					m.textarea.Focus()
					return m, nil
				}
				instructions := strings.TrimSpace(strings.TrimPrefix(input, "/compact"))
				m.messages = append(m.messages, messageItem{role: "system", content: "Compacting context...", ephemeral: true})
				m.refreshViewport()
				m.resetTextarea()
				m.textarea.Blur()
				return m, m.startCompaction(instructions)
			}

			// /model: open the model picker
			if input == "/model" {
				m.modelPickerActive = true
				m.modelPickerFilter = ""
				m.modelPickerIndex = 0
				m.setModelPickerIndexToCurrent()
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /theme: open the theme picker
			if input == "/theme" {
				m.themePickerActive = true
				m.themePickerFilter = ""
				m.themePickerIndex = 0
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /login: open the OAuth provider picker
			if input == "/login" {
				m.loginPickerActive = true
				m.loginPickerFilter = ""
				m.loginPickerIndex = 0
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /logout: open the logged-in OAuth provider picker
			if input == "/logout" {
				m.logoutPickerActive = true
				m.logoutPickerFilter = ""
				m.logoutPickerIndex = 0
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /resume: open the session picker.
			if input == "/resume" {
				m.resumePickerActive = true
				m.resumePickerFilter = ""
				m.resumePickerIndex = 0
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /new: start a fresh session
			if input == "/new" {
				if m.pending && m.agentCancel != nil {
					m.agentCancel()
				}
				m.ResetSession()
				m.messages = append(m.messages, messageItem{role: "system", content: "New session started.", ephemeral: true})
				m.refreshViewport()
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /session: show current session details.
			if input == "/session" {
				m.messages = append(m.messages, messageItem{role: "system", content: m.sessionInfoText(), ephemeral: true})
				m.refreshViewport()
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			// /export [path]: export current session as Markdown.
			if strings.HasPrefix(input, "/export") {
				path := strings.TrimSpace(strings.TrimPrefix(input, "/export"))
				msg := "Session export unavailable."
				if m.session != nil {
					if err := m.session.ExportMarkdown(path); err != nil {
						msg = "Export failed: " + err.Error()
					} else if path == "" {
						msg = "Exported session."
					} else {
						msg = "Exported session to " + path
					}
				}
				m.messages = append(m.messages, messageItem{role: "system", content: msg, ephemeral: true})
				m.refreshViewport()
				m.resetTextarea()
				m.textarea.Focus()
				return m, nil
			}

			m.resetTextarea()
			m.textarea.Blur()

			// Slash commands.
			if strings.HasPrefix(input, "/") {
				if m.isQuitCommand(input) {
					return m, tea.Quit
				}
				result, err := m.cmdReg.Execute(input)
				if err != nil {
					m.messages = append(m.messages, messageItem{role: "error", content: err.Error(), ephemeral: true})
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
				m.messages = append(m.messages, messageItem{role: "error", content: message, ephemeral: true})
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
				_ = m.session.SetTitleFromPrompt(input)
				_ = m.session.Append(session.Record{Role: "user", Content: input, Timestamp: time.Now()})
			}

			// Scroll to bottom when a new turn starts so the user sees
			// the agent response appear, even if they were reading history.
			m.viewport.GotoBottom()

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
	prevCursor := m.textareaCursorRune

	// Track cursor position for navigation keys before the textarea processes them.
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		runes := []rune(prevValue)
		switch {
		case keyMsg.Type == tea.KeyLeft || keyMsg.Type == tea.KeyCtrlB:
			if prevCursor > 0 {
				m.textareaCursorRune = prevCursor - 1
			}
		case keyMsg.Type == tea.KeyRight || keyMsg.Type == tea.KeyCtrlF:
			if prevCursor < len(runes) {
				m.textareaCursorRune = prevCursor + 1
			}
		case keyMsg.Type == tea.KeyHome || keyMsg.Type == tea.KeyCtrlA:
			// Start of current logical line.
			pos := -1
			for i := prevCursor - 1; i >= 0; i-- {
				if runes[i] == '\n' {
					pos = i + 1
					break
				}
			}
			if pos < 0 {
				pos = 0
			}
			m.textareaCursorRune = pos
		case keyMsg.Type == tea.KeyEnd || keyMsg.Type == tea.KeyCtrlE:
			// End of current logical line.
			pos := -1
			for i := prevCursor; i < len(runes); i++ {
				if runes[i] == '\n' {
					pos = i
					break
				}
			}
			if pos < 0 {
				pos = len(runes)
			}
			m.textareaCursorRune = pos
		case keyMsg.Type == tea.KeyUp || keyMsg.Type == tea.KeyCtrlP:
			m.textareaCursorRune = m.cursorMoveUp(runes, prevCursor)
		case keyMsg.Type == tea.KeyDown || keyMsg.Type == tea.KeyCtrlN:
			m.textareaCursorRune = m.cursorMoveDown(runes, prevCursor)
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)
	newValue := m.textarea.Value()

	// Detect and squash large pastes: if the textarea grew by more than a
	// single character, that's a paste. Squash it to a compact marker when
	// the pasted text exceeds 10 lines or 1000 characters.
	if newValue != prevValue && !m.modelPickerActive && !m.themePickerActive &&
		!m.loginPickerActive && !m.logoutPickerActive && !m.resumePickerActive {
		prevRunes := []rune(prevValue)
		newRunes := []rune(newValue)
		delta := len(newRunes) - len(prevRunes)
		end := prevCursor + delta
		if end > len(newRunes) {
			end = len(newRunes)
		}
		if end < 0 {
			end = 0
		}
		if delta > 1000 || (delta > 1 && strings.Count(string(newRunes[prevCursor:end]), "\n") >= 10) {
			// Extract the pasted text (inserted at cursor position).
			pastedRunes := newRunes[prevCursor:end]
			pastedText := string(pastedRunes)

			// Compute stats.
			lines := strings.Count(pastedText, "\n") + 1
			charCount := len(pastedRunes)

			// Store the full paste and increment counter.
			m.pasteCounter++
			id := m.pasteCounter
			m.pasteStore[id] = pastedText

			// Build a compact marker.
			var marker string
			if lines > 10 {
				marker = fmt.Sprintf("[paste #%d +%d lines]", id, lines)
			} else {
				marker = fmt.Sprintf("[paste #%d %d chars]", id, charCount)
			}

			// Replace the pasted text with the marker in the textarea.
			beforeRunes := newRunes[:prevCursor]
			afterRunes := newRunes[end:]
			newVal := string(beforeRunes) + marker + string(afterRunes)
			m.textarea.SetValue(newVal)

			// Move cursor past the marker.
			m.textareaCursorRune = prevCursor + len([]rune(marker))
		}
	}

	// Clear ephemeral messages when user edits the input (typing, backspace, delete, paste).
	if newValue != prevValue {
		m.clearEphemeralMessages()
		m.refreshViewport()
	}

	// Recalculate cursor position if value changed (typing, backspace, delete, paste).
	if newValue != prevValue {
		prevRunes := []rune(prevValue)
		newRunes := []rune(newValue)
		delta := len(newRunes) - len(prevRunes)
		if delta > 0 {
			// Typing or paste: cursor moved forward by inserted runes.
			m.textareaCursorRune = prevCursor + delta
		} else if delta < 0 {
			// Value shrank: backspace (cursor moves backward) or delete (cursor stays).
			if keyMsg, ok := msg.(tea.KeyMsg); ok && (keyMsg.Type == tea.KeyDelete || keyMsg.Type == tea.KeyCtrlD || keyMsg.Type == tea.KeyCtrlK) {
				// Delete/Ctrl+D (delete character forward), Ctrl+K (delete after cursor):
				// Cursor stays in place.
				m.textareaCursorRune = prevCursor
			} else {
				// Backspace, Ctrl+U (delete before cursor), Ctrl+W (delete word backward), etc.
				m.textareaCursorRune = prevCursor + delta
			}
		}
	}

	// Clamp cursor to valid range.
	if m.textareaCursorRune < 0 {
		m.textareaCursorRune = 0
	}
	maxRunes := len([]rune(m.textarea.Value()))
	if m.textareaCursorRune > maxRunes {
		m.textareaCursorRune = maxRunes
	}

	if m.ready && m.textarea.Height() != m.inputVisualLineCount() {
		m.textarea.SetHeight(m.inputVisualLineCount())
	}
	if m.modelPickerActive {
		prev := m.modelPickerFilter
		m.modelPickerFilter = m.textarea.Value()
		if m.modelPickerFilter != prev {
			m.modelPickerIndex = 0
		}
	}
	if m.themePickerActive {
		prev := m.themePickerFilter
		m.themePickerFilter = m.textarea.Value()
		if m.themePickerFilter != prev {
			m.themePickerIndex = 0
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
	if m.resumePickerActive {
		prev := m.resumePickerFilter
		m.resumePickerFilter = m.textarea.Value()
		if m.resumePickerFilter != prev {
			m.resumePickerIndex = 0
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

func (m Model) shouldInsertInputNewline(msg tea.Msg) bool {
	if m.pending || m.modelPickerActive || m.themePickerActive || m.loginPickerActive || m.logoutPickerActive || m.resumePickerActive {
		return false
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Ghostty can send LF for Shift+Enter; Bubble Tea v1 names LF as ctrl+j.
		// ESC+CR custom mappings are parsed by Bubble Tea v1 as alt+enter.
		return keyMsg.Type == tea.KeyCtrlJ || (keyMsg.Type == tea.KeyEnter && keyMsg.Alt)
	}

	return isShiftEnterSequence(rawMsgBytes(msg))
}

func (m Model) insertInputNewline() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	// Advance cursor past the inserted newline.
	m.textareaCursorRune++
	maxRunes := len([]rune(m.textarea.Value()))
	if m.textareaCursorRune > maxRunes {
		m.textareaCursorRune = maxRunes
	}
	// Update textarea height to accommodate new visual lines.
	if m.ready && m.textarea.Height() != m.inputVisualLineCount() {
		m.textarea.SetHeight(m.inputVisualLineCount())
	}
	return m, cmd
}

func rawMsgBytes(msg tea.Msg) []byte {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return nil
	}
	out := make([]byte, v.Len())
	reflect.Copy(reflect.ValueOf(out), v)
	return out
}

func isShiftEnterSequence(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	s := string(raw)
	if s == "\x1b\r" || s == "\x1b[13;2~" || s == "\x1b[27;2;13~" || s == "\n" {
		return true
	}
	// Kitty CSI-u: Enter is codepoint 13, keypad Enter is 57414, modifier 2 means Shift.
	return (strings.HasPrefix(s, "\x1b[13;2") || strings.HasPrefix(s, "\x1b[57414;2")) && strings.HasSuffix(s, "u")
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
		// Show the tool call immediately as it's being made by the model.
		if ev.ToolCallStart != nil {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				last.toolCalls = append(last.toolCalls, toolRenderItem{
					name:   ev.ToolCallStart.Name,
					callID: ev.ToolCallStart.CallID,
					state:  toolPending,
				})
				m.refreshViewport()
			}
		}

	case "tool_call_args":
		// Stream tool call args as they arrive so the user sees them building up.
		if ev.ToolCallArgs != "" {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				for i := len(last.toolCalls) - 1; i >= 0; i-- {
					if last.toolCalls[i].state == toolPending && last.toolCalls[i].name != "" {
						last.toolCalls[i].args += ev.ToolCallArgs
						break
					}
				}
				m.refreshViewport()
			}
		}

	case "tool_call_end":
		if ev.ToolCallEnd != nil {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				// Find the existing item created by tool_call_start and update its args.
				last := &m.messages[len(m.messages)-1]
				found := false
				for i := len(last.toolCalls) - 1; i >= 0; i-- {
					if last.toolCalls[i].callID == ev.ToolCallEnd.CallID {
						last.toolCalls[i].args = formatToolCallLine(ev.ToolCallEnd.Name, ev.ToolCallEnd.Args)
						last.toolCalls[i].rawArgs = ev.ToolCallEnd.Args
						found = true
						break
					}
				}
				if !found {
					last.toolCalls = append(last.toolCalls, toolRenderItem{
						name:    ev.ToolCallEnd.Name,
						callID:  ev.ToolCallEnd.CallID,
						args:    formatToolCallLine(ev.ToolCallEnd.Name, ev.ToolCallEnd.Args),
						rawArgs: ev.ToolCallEnd.Args,
						state:   toolPending,
					})
				}
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

	case "turn_usage":
		m.attachUsageToLastAssistant(ev.TurnUsage)
		m.refreshViewport()

	case "message_end":
		m.pending = false
		m.textarea.Focus()

		if ev.MessageEnd != nil {
			m.attachUsageToLastAssistant(ev.MessageEnd.Usage)

			if m.session != nil {
				rec := session.Record{
					Role:      "assistant",
					Content:   ev.MessageEnd.Text,
					Reasoning: ev.MessageEnd.ReasoningContent,
					Timestamp: time.Now(),
				}
				if rec.Content != "" || rec.Reasoning != "" {
					_ = m.session.Append(rec)
				}
			}
		}
		m.refreshViewport()

	case "error":
		if ev.Error != nil {
			m.pending = false
			m.textarea.Focus()
			m.messages = append(m.messages, messageItem{role: "error", content: ev.Error.Error(), ephemeral: true})
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
		filter := m.styles.Dim.Render("filter: ") + m.textarea.Value() + m.styles.InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else if m.themePickerActive {
		picker := m.renderThemePicker()
		filter := m.styles.Dim.Render("filter: ") + m.textarea.Value() + m.styles.InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else if m.loginPickerActive {
		picker := m.renderLoginPicker()
		filter := m.styles.Dim.Render("filter: ") + m.textarea.Value() + m.styles.InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else if m.logoutPickerActive {
		picker := m.renderLogoutPicker()
		filter := m.styles.Dim.Render("filter: ") + m.textarea.Value() + m.styles.InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else if m.resumePickerActive {
		picker := m.renderResumePicker()
		filter := m.styles.Dim.Render("filter: ") + m.textarea.Value() + m.styles.InputCursor.Render("█")
		if m.config.Alignment == "centered" {
			picker = centerContent(picker, m.width)
			filter = centerContent(filter, m.width)
		}
		b.WriteString(picker)
		b.WriteString("\n")
		b.WriteString(filter)
	} else {
		if m.pending {
			spinnerLine := m.spinner.View() + " " + m.styles.Dim.Render("Working")
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
		input := m.renderInputView()
		// Prepend "> " only to the first line.
		if idx := strings.Index(input, "\n"); idx >= 0 {
			input = "> " + input[:idx] + "\n" + input[idx+1:]
		} else {
			input = "> " + input
		}
		if m.config.Alignment == "centered" {
			input = centerContent(input, m.width)
		}
		b.WriteString(input)
	}
	b.WriteString("\n")
	cwd := m.styles.Dim.Render(compactCwd())
	sessionName := m.sessionDisplayName()
	if sessionName != "" {
		cwd += m.styles.Dim.Render("  ·  " + sessionName)
	}
	if m.config.Alignment == "centered" {
		cwd = centerContent(cwd, m.width)
	}
	b.WriteString(cwd)

	return b.String()
}

// clearEphemeralMessages removes all ephemeral messages from the message list.
func (m *Model) clearEphemeralMessages() {
	filtered := make([]messageItem, 0, len(m.messages))
	for _, msg := range m.messages {
		if !msg.ephemeral {
			filtered = append(filtered, msg)
		}
	}
	m.messages = filtered
}

func (m *Model) resetTextarea() {
	m.textarea.Reset()
	m.textarea.SetHeight(1)
	clear(m.pasteStore)
	m.pasteCounter = 0
}

// saveToHistory appends a non-empty input to the message history and resets
// the navigation position so the next Up press starts from the most recent entry.
func (m *Model) saveToHistory(input string) {
	m.messageHistory = append(m.messageHistory, input)
	m.messageHistoryPos = -1
	m.messageHistorySaved = ""
}

// visualLineRange holds the rune offset range of one visual (wrapped) line.
type visualLineRange struct {
	startRune int // rune offset of the first rune in this visual line
	endRune   int // rune offset past the last rune in this visual line (exclusive)
}

// buildVisualLines computes the visual line ranges for the given text.
// For each logical line, the text is split at width boundaries, producing
// one or more visual lines. Empty logical lines produce one empty visual line.
func buildVisualLines(runes []rune, width int) []visualLineRange {
	if width <= 0 {
		return []visualLineRange{{startRune: 0, endRune: len(runes)}}
	}

	var lines []visualLineRange
	i := 0
	for i < len(runes) {
		start := i
		// Advance to the end of this logical line (find \n or end).
		for i < len(runes) && runes[i] != '\n' {
			i++
		}
		logicalRunes := runes[start:i]
		// Skip the \n if present.
		if i < len(runes) && runes[i] == '\n' {
			i++
		}

		if len(logicalRunes) == 0 {
			// Empty logical line produces one empty visual line.
			lines = append(lines, visualLineRange{
				startRune: start,
				endRune:   start,
			})
			continue
		}

		// Wrap the logical line at width boundaries.
		remaining := logicalRunes
		runeOffset := start
		for len(remaining) > 0 {
			visStart := runeOffset
			visWidth := 0
			segLen := 0
			for _, r := range remaining {
				rw := runeDisplayWidth(r)
				if visWidth+rw > width && visWidth > 0 {
					break
				}
				visWidth += rw
				runeOffset++
				segLen++
			}
			lines = append(lines, visualLineRange{
				startRune: visStart,
				endRune:   runeOffset,
			})
			remaining = remaining[segLen:]
		}
	}

	// If the text ends with a newline, append an empty visual line.
	// Without this, "hello\n" renders as 1 line instead of 2.
	if len(runes) > 0 && runes[len(runes)-1] == '\n' {
		lines = append(lines, visualLineRange{
			startRune: len(runes),
			endRune:   len(runes),
		})
	}

	return lines
}

// cursorMoveUp returns the new cursor rune offset after moving up one visual line.
// It tries to maintain the horizontal column position.
func (m Model) cursorMoveUp(runes []rune, cursorRune int) int {
	return m.cursorMoveVertical(runes, cursorRune, -1)
}

// cursorMoveDown returns the new cursor rune offset after moving down one visual line.
// It tries to maintain the horizontal column position.
func (m Model) cursorMoveDown(runes []rune, cursorRune int) int {
	return m.cursorMoveVertical(runes, cursorRune, 1)
}

// cursorMoveVertical moves the cursor up (dir=-1) or down (dir=+1) one visual line,
// maintaining the horizontal column position as much as possible.
func (m Model) cursorMoveVertical(runes []rune, cursorRune int, dir int) int {
	width := m.textareaWidth()
	if width <= 0 || len(runes) == 0 {
		return cursorRune
	}

	lines := buildVisualLines(runes, width)
	if len(lines) == 0 {
		return cursorRune
	}

	// Find current visual line index.
	currentIdx := -1
	for i, vl := range lines {
		if cursorRune >= vl.startRune && cursorRune <= vl.endRune {
			currentIdx = i
			break
		}
	}
	if currentIdx < 0 {
		// Cursor past end of all lines — clamp to last visual line.
		currentIdx = len(lines) - 1
	}

	targetIdx := currentIdx + dir
	if targetIdx < 0 || targetIdx >= len(lines) {
		// Already at the top or bottom edge — stay where we are.
		return cursorRune
	}

	// Calculate the visual column of the cursor in the current visual line.
	col := 0
	for ri := lines[currentIdx].startRune; ri < cursorRune && ri < len(runes); ri++ {
		col += runeDisplayWidth(runes[ri])
	}

	// Move to the target visual line.
	targetLine := lines[targetIdx]
	newPos := targetLine.startRune

	// Walk forward in the target line by the same visual column.
	remainingCol := col
	for ri := targetLine.startRune; ri < targetLine.endRune && ri < len(runes); ri++ {
		rw := runeDisplayWidth(runes[ri])
		if remainingCol-rw < 0 {
			break
		}
		remainingCol -= rw
		newPos = ri + 1
	}

	return newPos
}

func (m Model) renderInputView() string {
	value := m.textarea.Value()
	if m.pending {
		return value
	}

	width := m.textareaWidth()
	if width <= 0 {
		return value
	}

	runes := []rune(value)

	if len(runes) == 0 {
		// Empty input — render the cursor block.
		m.textarea.Cursor.SetChar(" ")
		return m.textarea.Cursor.View()
	}

	// Build visual layout using shared helper.
	visualLines := buildVisualLines(runes, width)
	if len(visualLines) == 0 {
		m.textarea.Cursor.SetChar(" ")
		return m.textarea.Cursor.View()
	}

	// Find the visual position of the cursor based on tracked rune offset.
	cursorRune := m.textareaCursorRune
	if cursorRune < 0 {
		cursorRune = 0
	}
	if cursorRune > len(runes) {
		cursorRune = len(runes)
	}

	cursorVisIdx := len(visualLines) - 1
	cursorVisCol := 0
	for i, vl := range visualLines {
		if cursorRune >= vl.startRune && cursorRune <= vl.endRune {
			cursorVisIdx = i
			// Calculate visual column: width of runes from startRune to cursorRune.
			col := 0
			for ri := vl.startRune; ri < cursorRune && ri < len(runes); ri++ {
				col += runeDisplayWidth(runes[ri])
			}
			// Account for "  " indent on non-first visual lines.
			if i > 0 {
				col += 2
			}
			cursorVisCol = col
			break
		}
	}

	// Build each visual line's text.
	var lineTexts []string
	for i, vl := range visualLines {
		text := string(runes[vl.startRune:vl.endRune])
		if i > 0 {
			text = "  " + text
		}
		lineTexts = append(lineTexts, text)
	}

	// Build output with cursor rendered as a block cursor on top of the
	// character at the cursor position (or a space at the end of text).
	var out strings.Builder
	for i, text := range lineTexts {
		if i == cursorVisIdx {
			// Find the rune at the cursor visual column within this line.
			col := 0
			pos := 0
			trunes := []rune(text)
			for _, r := range trunes {
				if col >= cursorVisCol {
					break
				}
				col += runeDisplayWidth(r)
				pos++
			}
			bytePos := len(string(trunes[:pos]))
			// Determine the character at cursor position.
			if pos < len(trunes) {
				cursorChar := string(trunes[pos])
				m.textarea.Cursor.SetChar(cursorChar)
				out.WriteString(text[:bytePos])
				out.WriteString(m.textarea.Cursor.View())
				out.WriteString(text[bytePos+len(cursorChar):])
			} else {
				// Cursor past end of line — show block cursor.
				m.textarea.Cursor.SetChar(" ")
				out.WriteString(text)
				out.WriteString(m.textarea.Cursor.View())
			}
		} else {
			out.WriteString(text)
		}
		if i < len(lineTexts)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func (m Model) renderStatusLine() string {
	providerName := valueOrDefault(m.config.Provider, "unknown")
	modelName := valueOrDefault(m.config.Model, "unknown")
	thinking := valueOrDefault(m.config.Thinking, "none")
	line := fmt.Sprintf("%s | %s | %s | %s/%s | %s", providerName, modelName, thinking, formatTokenCount(m.estimatedContextUsed()), formatTokenCount(m.maxContextForCurrentModel()), m.renderCostStatus())
	if m.width > 0 {
		line = truncatePlainLine(line, m.width)
	}
	return m.styles.Dim.Render(line)
}

func truncatePlainLine(line string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(line) <= maxWidth {
		return line
	}
	if maxWidth == 1 {
		return "…"
	}
	runes := []rune(line)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func (m Model) estimatedContextUsed() int {
	chars := len(prompt.Build(*m.config, getCwd(), m.skills))

	if m.toolReg != nil {
		if toolBytes, err := json.Marshal(m.toolReg.ToProviderTools()); err == nil {
			chars += len(toolBytes)
		}
	}

	for _, msg := range m.messages {
		if msg.role != "assistant" && msg.role != "user" && msg.ephemeral {
			continue
		}
		switch msg.role {
		case "user", "system", "compaction":
			chars += len(msg.role) + len(msg.content)
		case "assistant":
			chars += len(msg.role) + len(msg.content) + len(msg.reasoning)
			for _, tc := range msg.toolCalls {
				if tc.callID != "" {
					chars += len(tc.name) + len(tc.callID)
					if argsBytes, err := json.Marshal(tc.rawArgs); err == nil {
						chars += len(argsBytes)
					} else {
						chars += len(tc.args)
					}
				}
				if tc.output != "" {
					chars += len("tool") + len(tc.callID) + len(tc.output)
				}
			}
		}
	}

	if input := strings.TrimSpace(m.textarea.Value()); input != "" && !m.pending {
		chars += len("user") + len(input)
	}

	return (chars + 3) / 4
}

func (m *Model) attachUsageToLastAssistant(usage *provider.Usage) {
	if usage == nil || len(m.messages) == 0 || m.messages[len(m.messages)-1].role != "assistant" {
		return
	}
	m.calculateUsageCost(usage)
	m.messages[len(m.messages)-1].usageData = usage
}

func (m Model) renderCostStatus() string {
	cost, subscription := m.totalUsageCost()
	if subscription {
		return "sub"
	}
	return fmt.Sprintf("$%.4f", cost)
}

func (m Model) totalUsageCost() (float64, bool) {
	subscription := provider.IsSubscriptionProvider(m.config.Provider)
	var total float64
	for _, msg := range m.messages {
		if msg.usageData == nil {
			continue
		}
		if msg.usageData.Subscription {
			subscription = true
		}
		total += msg.usageData.Cost.Total
	}
	return total, subscription
}

func (m Model) calculateUsageCost(usage *provider.Usage) {
	pricing := m.pricingForCurrentModel()
	provider.CalculateCost(usage, pricing, provider.IsSubscriptionProvider(m.config.Provider))
}

func (m Model) pricingForCurrentModel() provider.Pricing {
	if m.modelReg != nil {
		for _, model := range m.modelReg.GetAll() {
			if model.ID == m.config.Model && model.Provider == m.config.Provider && hasPricing(model.Pricing) {
				return provider.Pricing{
					InputPerMTok:      model.Pricing.InputPerMTok,
					OutputPerMTok:     model.Pricing.OutputPerMTok,
					CacheReadPerMTok:  model.Pricing.CacheReadPerMTok,
					CacheWritePerMTok: model.Pricing.CacheWritePerMTok,
				}
			}
		}
		for _, model := range m.modelReg.GetAll() {
			if model.ID == m.config.Model && hasPricing(model.Pricing) {
				return provider.Pricing{
					InputPerMTok:      model.Pricing.InputPerMTok,
					OutputPerMTok:     model.Pricing.OutputPerMTok,
					CacheReadPerMTok:  model.Pricing.CacheReadPerMTok,
					CacheWritePerMTok: model.Pricing.CacheWritePerMTok,
				}
			}
		}
	}
	return provider.PricingForModel(m.config.Provider, m.config.Model)
}

func hasPricing(pricing commands.Pricing) bool {
	return pricing.InputPerMTok != 0 || pricing.OutputPerMTok != 0 || pricing.CacheReadPerMTok != 0 || pricing.CacheWritePerMTok != 0
}

func (m Model) maxContextForCurrentModel() int {
	if m.modelReg != nil {
		for _, model := range m.modelReg.GetAll() {
			if model.ID == m.config.Model && model.Provider == m.config.Provider && model.ContextLength > 0 {
				return model.ContextLength
			}
		}
		for _, model := range m.modelReg.GetAll() {
			if model.ID == m.config.Model && model.ContextLength > 0 {
				return model.ContextLength
			}
		}
	}
	return provider.ContextWindowForModel(m.config.Provider, m.config.Model)
}

func formatTokenCount(tokens int) string {
	if tokens >= 1_000_000 {
		if tokens%1_000_000 == 0 {
			return fmt.Sprintf("%dM", tokens/1_000_000)
		}
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1_000 {
		if tokens%1_000 == 0 {
			return fmt.Sprintf("%dk", tokens/1_000)
		}
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
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
	val := "/" + selected.Name + " "
	m.textarea.SetValue(val)
	m.textareaCursorRune = len([]rune(val))
	m.textarea.CursorEnd()
	m.commandSuggestionIndex = 0
}

func (m Model) filteredSessions() []session.SessionInfo {
	infos, err := session.List(m.config.SessionDir)
	if err != nil {
		return nil
	}
	filter := strings.ToLower(strings.TrimSpace(m.resumePickerFilter))
	out := make([]session.SessionInfo, 0, len(infos))
	for _, info := range infos {
		if info.MessageCount == 0 && filter == "" {
			continue
		}
		if filter == "" || sessionMatchesFilter(info, filter) {
			out = append(out, info)
		}
	}
	if len(out) == 0 && filter == "" {
		return infos
	}
	return out
}

func sessionMatchesFilter(info session.SessionInfo, filter string) bool {
	fields := []string{info.Title, info.FirstPrompt, info.FirstMessage, info.ID, info.Path, info.CWD}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), filter) {
			return true
		}
	}
	return false
}

func (m Model) handleResumePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	sessions := m.filteredSessions()

	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.resumePickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEnter, tea.KeyTab:
		if len(sessions) > 0 {
			m.clampResumePickerIndex(sessions)
			m.resumeSession(sessions[m.resumePickerIndex])
		}
		m.resumePickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyUp:
		if len(sessions) > 0 {
			m.resumePickerIndex--
			m.clampResumePickerIndex(sessions)
		}
		return m, nil, true
	case tea.KeyDown:
		if len(sessions) > 0 {
			m.resumePickerIndex++
			m.clampResumePickerIndex(sessions)
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) clampResumePickerIndex(infos []session.SessionInfo) {
	if len(infos) == 0 {
		m.resumePickerIndex = 0
		return
	}
	if m.resumePickerIndex < 0 {
		m.resumePickerIndex = 0
	}
	if m.resumePickerIndex >= len(infos) {
		m.resumePickerIndex = len(infos) - 1
	}
}

func (m Model) renderResumePicker() string {
	sessions := m.filteredSessions()
	var b strings.Builder
	if len(sessions) == 0 {
		b.WriteString(m.styles.Dim.Render("  No sessions match your filter"))
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  sessions (%d)", len(sessions))))
		b.WriteString("\n")
		m.clampResumePickerIndex(sessions)
		start, end, _ := m.visibleSessionPickerRange(sessions)
		if start > 0 {
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more above", start)))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			info := sessions[i]
			prefix := "  "
			style := m.styles.Dim
			if i == m.resumePickerIndex {
				prefix = "> "
				style = m.styles.UserPrompt
			}
			title := sessionDisplayTitle(info)
			if len(title) > 58 {
				title = title[:57] + "…"
			}
			line := fmt.Sprintf("%s%-60s %7s  %d msgs", prefix, title, relativeTime(info.Modified), info.MessageCount)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
		if end < len(sessions) {
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more below", len(sessions)-end)))
			b.WriteString("\n")
		}
	}
	b.WriteString(m.styles.Dim.Render("  esc: cancel  enter: resume  arrows: navigate  type: filter"))
	return b.String()
}

func sessionDisplayTitle(info session.SessionInfo) string {
	for _, value := range []string{info.Title, info.FirstPrompt, info.FirstMessage} {
		if strings.TrimSpace(value) != "" && value != "(no messages)" {
			return strings.TrimSpace(value)
		}
	}
	return "(empty session)"
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	if d < 30*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
}

// handleModelPickerKey processes key events when model picker is active.
// Returns (model, cmd, handled). If handled is false, the event falls through
// to the textarea for character/backspace input.
func (m Model) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	models := m.cmdReg.FilterModels(m.modelPickerFilter)

	switch msg.Type {
	case tea.KeyCtrlC:
		m.modelPickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyEsc:
		m.modelPickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyEnter:
		if len(models) > 0 {
			m.clampModelPickerIndex(models)
			selected := models[m.modelPickerIndex]
			m.selectModel(selected.ModelProvider, selected.ModelID)
		}
		m.modelPickerActive = false
		m.resetTextarea()
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
		m.resetTextarea()
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

func (m *Model) setModelPickerIndexToCurrent() {
	if m.cmdReg == nil || m.config == nil || m.config.Provider == "" || m.config.Model == "" {
		return
	}
	models := m.cmdReg.FilterModels(m.modelPickerFilter)
	for i, model := range models {
		if model.ModelProvider == m.config.Provider && model.ModelID == m.config.Model {
			m.modelPickerIndex = i
			return
		}
	}
}

// handleThemePickerKey processes key events when theme picker is active.
// Returns (model, cmd, handled). If handled is false, the event falls through
// to the textarea for character/backspace input.
func (m Model) handleThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	themes := m.filteredThemes()

	switch msg.Type {
	case tea.KeyCtrlC:
		m.themePickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyEsc:
		m.themePickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyEnter:
		if len(themes) > 0 {
			m.clampThemePickerIndex(themes)
			selected := themes[m.themePickerIndex]
			m.selectTheme(selected.Name)
		}
		m.themePickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true

	case tea.KeyUp:
		if len(themes) > 0 {
			m.themePickerIndex--
			m.clampThemePickerIndex(themes)
		}
		return m, nil, true

	case tea.KeyDown:
		if len(themes) > 0 {
			m.themePickerIndex++
			m.clampThemePickerIndex(themes)
		}
		return m, nil, true

	case tea.KeyTab:
		if len(themes) > 0 {
			m.clampThemePickerIndex(themes)
			selected := themes[m.themePickerIndex]
			m.selectTheme(selected.Name)
		}
		m.themePickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	}

	return m, nil, false
}

func (m Model) filteredThemes() []themes.Info {
	infos, err := themes.AvailableThemes()
	if err != nil {
		return themes.BuiltinThemes()
	}
	filter := strings.ToLower(strings.TrimSpace(m.themePickerFilter))
	if filter == "" {
		return infos
	}
	out := make([]themes.Info, 0, len(infos))
	for _, info := range infos {
		if strings.Contains(strings.ToLower(info.Name), filter) || strings.Contains(strings.ToLower(info.Description), filter) {
			out = append(out, info)
		}
	}
	return out
}

func (m *Model) clampThemePickerIndex(infos []themes.Info) {
	if len(infos) == 0 {
		m.themePickerIndex = 0
		return
	}
	if m.themePickerIndex < 0 {
		m.themePickerIndex = 0
	}
	if m.themePickerIndex >= len(infos) {
		m.themePickerIndex = len(infos) - 1
	}
}

// renderModelPicker renders the model picker modal.
func (m Model) renderModelPicker() string {
	models := m.cmdReg.FilterModels(m.modelPickerFilter)

	var b strings.Builder

	if len(models) == 0 {
		b.WriteString(m.styles.Dim.Render("  No models match your filter"))
		if m.modelPickerFilter == "" {
			b.WriteString(m.styles.Dim.Render(" (no models available)"))
		}
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  models (%d)", len(models))))
		b.WriteString("\n")

		m.clampModelPickerIndex(models)
		start, end, _ := m.visibleModelPickerRange(models)

		if start > 0 {
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more above", start)))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			mdl := models[i]
			prefix := "  "
			style := m.styles.Dim
			if i == m.modelPickerIndex {
				prefix = "> "
				style = m.styles.UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, mdl.ModelID)
			if mdl.Args != "" {
				line += m.styles.Dim.Render(fmt.Sprintf("  (%s)", mdl.Args))
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
		if end < len(models) {
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more below", len(models)-end)))
			b.WriteString("\n")
		}
	}

	b.WriteString(m.styles.Dim.Render("  esc: cancel  enter: select  arrows: navigate  type: filter"))

	return b.String()
}

// renderThemePicker renders the theme picker modal.
func (m Model) renderThemePicker() string {
	infos := m.filteredThemes()

	var b strings.Builder

	if len(infos) == 0 {
		b.WriteString(m.styles.Dim.Render("  No themes match your filter"))
		if m.themePickerFilter == "" {
			b.WriteString(m.styles.Dim.Render(" (no themes available)"))
		}
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  themes (%d)", len(infos))))
		b.WriteString("\n")

		m.clampThemePickerIndex(infos)
		start, end, _ := m.visibleThemePickerRange(infos)

		if start > 0 {
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more above", start)))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			info := infos[i]
			prefix := "  "
			style := m.styles.Dim
			if i == m.themePickerIndex {
				prefix = "> "
				style = m.styles.UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, info.Name)
			if info.Description != "" {
				line += m.styles.Dim.Render(fmt.Sprintf("  (%s)", info.Description))
			}
			if info.Name == m.config.Theme || (m.config.Theme == "" && info.Name == "crobot-dark") {
				line += m.styles.Dim.Render("  current")
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
		if end < len(infos) {
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more below", len(infos)-end)))
			b.WriteString("\n")
		}
	}

	b.WriteString(m.styles.Dim.Render("  esc: cancel  enter: select  arrows: navigate  type: filter"))

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
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEsc:
		m.loginPickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEnter, tea.KeyTab:
		if len(providers) == 0 {
			return m, nil, true
		}
		m.clampLoginPickerIndex(providers)
		selected := providers[m.loginPickerIndex]
		m.loginPickerActive = false
		m.resetTextarea()
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
	for _, providerName := range []string{"openrouter", "openai", "openai-responses-ws", "openai-codex", "deepseek", "anthropic"} {
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
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEsc:
		m.logoutPickerActive = false
		m.resetTextarea()
		m.textarea.Focus()
		return m, nil, true
	case tea.KeyEnter, tea.KeyTab:
		if len(providers) == 0 {
			return m, nil, true
		}
		m.clampLogoutPickerIndex(providers)
		selected := providers[m.logoutPickerIndex]
		m.logoutPickerActive = false
		m.resetTextarea()
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
		b.WriteString(m.styles.Dim.Render("  No logged-in OAuth providers"))
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  logged-in OAuth providers (%d)", len(providers))))
		b.WriteString("\n")
		m.clampLogoutPickerIndex(providers)
		for i, p := range providers {
			prefix := "  "
			style := m.styles.Dim
			if i == m.logoutPickerIndex {
				prefix = "> "
				style = m.styles.UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, p.Name)
			if p.Description != "" {
				line += m.styles.Dim.Render(fmt.Sprintf("  (%s)", p.Description))
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
	}
	b.WriteString(m.styles.Dim.Render("  esc: cancel  enter: logout  arrows: navigate  type: filter"))
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
		b.WriteString(m.styles.Dim.Render("  No OAuth providers match your filter"))
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  OAuth providers (%d)", len(providers))))
		b.WriteString("\n")
		m.clampLoginPickerIndex(providers)
		for i, p := range providers {
			prefix := "  "
			style := m.styles.Dim
			if i == m.loginPickerIndex {
				prefix = "> "
				style = m.styles.UserPrompt
			}
			line := fmt.Sprintf("%s%s", prefix, p.Name)
			if p.Description != "" {
				line += m.styles.Dim.Render(fmt.Sprintf("  (%s)", p.Description))
			}
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
	}
	b.WriteString(m.styles.Dim.Render("  esc: cancel  enter: login  arrows: navigate  type: filter"))
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

func (m Model) visibleSessionPickerRange(infos []session.SessionInfo) (start, end, selected int) {
	const maxVisible = 12

	selected = m.resumePickerIndex
	if selected >= len(infos) {
		selected = len(infos) - 1
	}
	if selected < 0 {
		selected = 0
	}

	end = len(infos)
	if len(infos) > maxVisible {
		start = selected - maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + maxVisible
		if end > len(infos) {
			end = len(infos)
			start = end - maxVisible
		}
	}
	return start, end, selected
}

func (m Model) visibleThemePickerRange(infos []themes.Info) (start, end, selected int) {
	const maxVisible = 12

	selected = m.themePickerIndex
	if selected >= len(infos) {
		selected = len(infos) - 1
	}
	if selected < 0 {
		selected = 0
	}

	end = len(infos)
	if len(infos) > maxVisible {
		start = selected - maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + maxVisible
		if end > len(infos) {
			end = len(infos)
			start = end - maxVisible
		}
	}
	return start, end, selected
}

func (m *Model) resumeSession(info session.SessionInfo) {
	if m.pending && m.agentCancel != nil {
		m.agentCancel()
		m.agentCancel = nil
	}
	m.pending = false
	sess, err := session.Open(info.Path)
	if err != nil {
		m.messages = append(m.messages, messageItem{role: "error", content: "Resume failed: " + err.Error(), ephemeral: true})
		m.refreshViewport()
		return
	}
	m.session = sess
	records, err := sess.Load()
	if err != nil {
		m.messages = append(m.messages, messageItem{role: "error", content: "Resume failed: " + err.Error(), ephemeral: true})
		m.refreshViewport()
		return
	}
	m.messages = nil
	for _, rec := range records {
		if rec.Role == "user" || rec.Role == "assistant" {
			m.messages = append(m.messages, messageItem{role: rec.Role, content: rec.Content, reasoning: rec.Reasoning})
		}
	}
	m.previousCompactionSummary = ""
	m.selection.clear()
	m.commandSuggestionIndex = 0
	m.messages = append(m.messages, messageItem{role: "system", content: "Resumed: " + sessionDisplayTitle(info), ephemeral: true})
	m.refreshViewport()
}

func (m *Model) sessionInfoText() string {
	if m.session == nil {
		return "Session: disabled"
	}
	info, err := m.session.Info()
	if err != nil {
		return "Session: " + m.session.Path()
	}
	return fmt.Sprintf("Session: %s\nPath: %s\nMessages: %d\nCreated: %s\nModified: %s\nFirst message: %s",
		m.session.ID(),
		info.Path,
		info.MessageCount,
		info.Created.Format(time.RFC3339),
		info.Modified.Format(time.RFC3339),
		info.FirstMessage,
	)
}

// ResetSession clears all conversation state and creates a new session file.
// This is effectively like restarting the app without exiting.
func (m *Model) ResetSession() {
	// Cancel any pending agent. The agent goroutine's old event channel
	// will close naturally when the agent exits. Any stale agentDoneMsg
	// that arrives later is harmless (m.pending is already false, messages
	// are empty, so auto-compaction is a no-op).
	if m.agentCancel != nil {
		m.agentCancel()
		m.agentCancel = nil
	}
	m.pending = false

	// Create a new session file with a fresh ID when persistence is configured.
	m.session = nil
	if strings.TrimSpace(m.config.SessionDir) != "" {
		cwd, _ := os.Getwd()
		sess, err := session.Create(m.config.SessionDir, cwd)
		if err == nil {
			m.session = sess
		}
	}

	// Clear messages, compaction state, input, and selection.
	m.messages = nil
	m.previousCompactionSummary = ""
	m.resetTextarea()
	m.selection.clear()
	m.commandSuggestionIndex = 0
}

func (m *Model) selectTheme(name string) {
	if name == "" {
		return
	}
	th, err := themes.LoadTheme(name)
	if err != nil {
		m.messages = append(m.messages, messageItem{role: "error", content: fmt.Sprintf("Theme %q failed to load: %v", name, err), ephemeral: true})
		m.refreshViewport()
		return
	}
	m.config.Theme = name
	_ = config.SaveConfig(m.config)
	m.styles = NewStyles(th)
	SetLoaderSpinnerStyle(&m.spinner, m.styles)
	m.messages = append(m.messages, messageItem{role: "system", content: fmt.Sprintf("Theme set to: %s", name), ephemeral: true})
	m.refreshViewport()
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

	// Record model in history.
	if m.modelHistory != nil {
		m.modelHistory.Record(providerName, modelID)
	}

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

	m.resetTextarea()
	m.commandSuggestionIndex = 0
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
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("models (%d)", len(suggestions))))
	} else {
		b.WriteString(m.styles.Dim.Render("commands"))
	}

	if start > 0 {
		b.WriteString("\n")
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more above", start)))
	}
	for i := start; i < end; i++ {
		cmd := suggestions[i]
		prefix := "  "
		style := m.styles.Dim
		if i == selected {
			prefix = "> "
			style = m.styles.UserPrompt
		}

		var line string
		if isModel {
			line = fmt.Sprintf("%s%s", prefix, cmd.ModelID)
			if cmd.Args != "" {
				line += m.styles.Dim.Render(fmt.Sprintf("  (%s)", cmd.Args))
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
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("  +%d more below", len(suggestions)-end)))
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

func (m Model) themePickerHeight() int {
	infos := m.filteredThemes()
	if len(infos) == 0 {
		return 2 // empty message + help line
	}
	start, end, _ := m.visibleThemePickerRange(infos)
	height := 1 + end - start // header + visible themes
	if start > 0 {
		height++
	}
	if end < len(infos) {
		height++
	}
	return height + 1 // help line
}

func (m Model) dynamicViewportHeight() int {
	footerHeight := 4 + m.textarea.Height() + m.commandSuggestionHeight()
	if m.modelPickerActive {
		footerHeight = 3 + m.modelPickerHeight()
	}
	if m.themePickerActive {
		footerHeight = 3 + m.themePickerHeight()
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
	if m.config.Alignment == "centered" {
		wrapWidth = wrapWidth * 3 / 4
	}
	if wrapWidth < 40 {
		wrapWidth = 40
	}
	if m.config.ShowBanner {
		b.WriteString(Render(m.config.Model, version.Version))
		b.WriteString("\n")
	}
	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString("  ")
			b.WriteString(m.styles.UserCaret.Render(">"))
			b.WriteString(" ")
			b.WriteString(m.styles.UserPrompt.Render(wrapText(msg.content, wrapWidth-4)))
			b.WriteString("\n\n")
		case "assistant":
			if msg.reasoning != "" && m.config.Reasoning {
				b.WriteString(m.styles.ThinkingStyle.Render("thinking"))
				b.WriteString("\n")
				b.WriteString(m.styles.ThinkingStyle.Render(wrapText(msg.reasoning, wrapWidth)))
				b.WriteString("\n")
			}
			if msg.content != "" {
				b.WriteString(RenderMarkdown(msg.content, wrapWidth, m.styles))
			}
			for _, tc := range msg.toolCalls {
				tcWidth := m.width - 4
				if m.config.Alignment == "centered" {
					tcWidth = m.width * 3 / 4
					if tcWidth < 40 {
						tcWidth = 40
					}
				}
				b.WriteString(RenderToolCall(tc, tcWidth, m.toolOutputExpanded, m.styles))
				b.WriteString("\n")
			}
		case "system":
			b.WriteString(m.styles.Dim.Render(wrapText(msg.content, wrapWidth)))
			b.WriteString("\n\n")
		case "compaction":
			b.WriteString(m.styles.Dim.Render("[compaction] "))
			b.WriteString(m.styles.Dim.Render(wrapText(msg.content, wrapWidth)))
			b.WriteString("\n\n")
		case "error":
			errWidth := wrapWidth - 7
			if errWidth < 20 {
				errWidth = 20
			}
			b.WriteString(m.styles.Red.Render("Error: " + wrapText(msg.content, errWidth)))
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
	// Check AtBottom before changing Height so the check uses the same
	// Height that any prior GotoBottom() used to set YOffset. Otherwise
	// a Height change (e.g. when pending becomes true and the viewport
	// loses 1 line) makes AtBottom() return false even though we just
	// forced a scroll-to-bottom, causing the viewport to drift above the
	// bottom during agent streaming.
	wasAtBottom := m.viewport.AtBottom()
	m.viewport.Height = m.dynamicViewportHeight()
	content := m.renderViewportContent()
	m.viewport.SetContent(content)
	m.plainLines = strings.Split(stripANSI(content), "\n")
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// inputVisualLineCount returns the number of visual lines the textarea value needs.
// Lines wrap at the textarea width.
func (m Model) inputVisualLineCount() int {
	value := m.textarea.Value()
	if value == "" {
		return 1
	}
	width := m.textareaWidth()
	if width <= 0 {
		return 1
	}

	lines := strings.Split(value, "\n")
	count := 0
	for _, logicalLine := range lines {
		if logicalLine == "" {
			count++
			continue
		}
		vl := lipgloss.Width(logicalLine)
		if vl == 0 {
			count++
			continue
		}
		n := (vl + width - 1) / width
		if n < 1 {
			n = 1
		}
		count += n
	}
	if count < 1 {
		count = 1
	}
	const maxLines = 5
	if count > maxLines {
		count = maxLines
	}
	return count
}

func (m Model) textareaWidth() int {
	w := m.width - 4
	if w < 20 {
		w = 20
	}
	return w
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
// Uses manual padding via lipgloss.Width instead of Style.Align(Center)
// because Align strips leading/trailing whitespace, which misbehaves when
// ANSI escape codes appear before whitespace on a line.
func centerContent(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		visualWidth := lipgloss.Width(line)
		if visualWidth >= width {
			continue
		}
		padding := (width - visualWidth) / 2
		lines[i] = strings.Repeat(" ", padding) + line + strings.Repeat(" ", width-padding-visualWidth)
	}
	return strings.Join(lines, "\n")
}

// --- Agent runner goroutine ---

// messageToConversation converts a TUI messageItem to the canonical conversation message.
func messageToConversation(msg messageItem) conversation.Message {
	result := conversation.Message{
		Role:      msg.role,
		Content:   msg.content,
		Reasoning: msg.reasoning,
		Usage:     msg.usageData,
		ToolCalls: make([]conversation.ToolResult, len(msg.toolCalls)),
	}
	for i, tc := range msg.toolCalls {
		result.ToolCalls[i] = conversation.ToolResult{
			Name:    tc.name,
			CallID:  tc.callID,
			Output:  tc.output,
			ArgsStr: tc.args,
			Args:    tc.rawArgs,
		}
	}
	return result
}

// compactionToMessageItem converts a compaction message to a TUI messageItem.
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
		m.messages = append(m.messages, messageItem{role: "error", content: fmt.Sprintf("Compaction failed: %v", msg.err), ephemeral: true})
		m.refreshViewport()
		m.resetTextarea()
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
	m.resetTextarea()
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

// messagesToConversation converts TUI messages to canonical conversation messages.
func messagesToConversation(msgs []messageItem, includeEphemeral bool) []conversation.Message {
	result := make([]conversation.Message, 0, len(msgs))
	for _, msg := range msgs {
		// Skip ephemeral messages and non-conversation roles (error, system) when building
		// the LLM-facing conversation history. Only user and assistant messages represent
		// actual dialogue. Error and system messages are UI-only annotations.
		if !includeEphemeral && (msg.ephemeral || msg.role == "error" || msg.role == "system") {
			continue
		}
		result = append(result, messageToConversation(msg))
	}
	return result
}

// messagesToCompaction converts TUI messages to compaction messages.
func messagesToCompaction(msgs []messageItem) []compaction.MessageItem {
	result := make([]compaction.MessageItem, 0, len(msgs))
	for _, msg := range msgs {
		compMsg := compaction.MessageItem{
			Role:      msg.role,
			Content:   msg.content,
			Reasoning: msg.reasoning,
			ToolCalls: make([]compaction.ToolRenderItem, len(msg.toolCalls)),
		}
		for i, tc := range msg.toolCalls {
			compMsg.ToolCalls[i] = compaction.ToolRenderItem{
				Name:    tc.name,
				CallID:  tc.callID,
				Output:  tc.output,
				Args:    tc.args,
				RawArgs: tc.rawArgs,
			}
		}
		result = append(result, compMsg)
	}
	return result
}

func (m *Model) startAgent(ctx context.Context, input string) {
	// Capture the channel locally so this goroutine only ever touches
	// its own instance, even if m.agentEvents is reassigned later.
	ch := m.agentEvents
	defer close(ch)

	// The latest user message is already in m.messages.
	// Run the frontend-agnostic backend loop.
	_, _ = runtime.RunAgent(ctx, runtime.AgentRequest{
		Config:   m.config,
		Provider: m.provider,
		ToolReg:  m.toolReg,
		Plugins:  m.plugins,
		Skills:   m.skills,
		CWD:      getCwd(),
		Messages: messagesToConversation(m.messages, false),
		OnEvent: func(ev agent.Event) {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		},
	})
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

// expandPasteMarkers expands any paste markers in the input back to their
// original full text. Markers have the form [paste #N +M lines] or [paste #N M chars].
func (m *Model) expandPasteMarkers(input string) string {
	if len(m.pasteStore) == 0 {
		return input
	}
	re := regexp.MustCompile(`\[paste #(\d+) [^]]+\]`)
	return re.ReplaceAllStringFunc(input, func(marker string) string {
		matches := re.FindStringSubmatch(marker)
		if len(matches) < 2 {
			return marker
		}
		id, err := strconv.Atoi(matches[1])
		if err != nil {
			return marker
		}
		if text, ok := m.pasteStore[id]; ok {
			return text
		}
		return marker
	})
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

// sessionDisplayName returns the session title if available, or "New session"
// if the session has no name yet. Returns empty string if sessions are disabled.
func (m Model) sessionDisplayName() string {
	if m.session == nil {
		return ""
	}
	info, err := m.session.Info()
	if err != nil {
		return "New session"
	}
	if info.Title == "" {
		return "New session"
	}
	return info.Title
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
// call arguments, without the tool name (formatSingleToolCallLine prepends it).
func formatToolCallLine(name string, args map[string]any) string {
	if args == nil {
		return ""
	}
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			return "$ " + shortenPathsInCommand(cmd)
		}
		return ""
	case "file_read", "read":
		return formatFilePathCall(args, "path", "offset", "limit")
	case "file_write", "write":
		return formatFilePathCall(args, "path", "", "")
	case "file_edit", "edit":
		return formatFilePathCall(args, "path", "", "")
	case "grep":
		path, _ := args["path"].(string)
		pattern, _ := args["pattern"].(string)
		var b strings.Builder
		if pattern != "" {
			b.WriteString("/")
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
		if glob != "" {
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
			return shortenDisplayPath(path)
		}
		return ""
	default:
		key := summarizeKey(name)
		if v, ok := args[key]; ok {
			val := fmt.Sprintf("%v", v)
			if len(val) > 60 {
				val = val[:60] + "..."
			}
			return val
		}
		return ""
	}
}

// formatFilePathCall formats a file path with optional line range:
//
//	/to/file.go:1-20
//	/to/file.go
func formatFilePathCall(args map[string]any, pathKey, offsetKey, limitKey string) string {
	path, _ := args[pathKey].(string)
	if path == "" {
		return ""
	}
	short := shortenDisplayPath(path)

	offset := getIntArg(args, offsetKey, "offset", "start_line")
	limit := getIntArg(args, limitKey, "limit", "end_line")

	if offset > 0 || limit > 0 {
		start := offset
		if start <= 0 {
			start = 1
		}
		if limit > 0 {
			return fmt.Sprintf("%s:%d-%d", short, start, start+limit-1)
		}
		return fmt.Sprintf("%s:%d", short, start)
	}
	return short
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

// shortenPathsInCommand replaces absolute file paths in a bash command string
// with shortened versions using shortenDisplayPath. Only paths that exist under
// or equal to the CWD are shortened.
func shortenPathsInCommand(cmd string) string {
	cwd := getCwd()
	fields := strings.Fields(cmd)
	for i, f := range fields {
		// Strip common trailing punctuation that may be attached to a path.
		cleaned := strings.TrimRight(f, ",.;:\"'})]>!")
		if !strings.HasPrefix(cleaned, "/") {
			continue
		}
		if strings.HasPrefix(cleaned, cwd) {
			short := shortenDisplayPath(cleaned)
			// Preserve any trailing punctuation
			if short != cleaned {
				fields[i] = short + f[len(cleaned):]
			}
			continue
		}
		// Check if the path starts with CWD but has non-CWD prefix like /home/user/...
		// Actually we already check HasPrefix(cwd) above, so this covers all paths
		// under CWD. Paths outside CWD are left as-is.
	}
	return strings.Join(fields, " ")
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

// runeDisplayWidth returns the display width of a rune (1 for normal,
// 2 for CJK wide characters, 0 for zero-width characters).
func runeDisplayWidth(r rune) int {
	// Use go-runewidth for accurate width measurement.
	// This handles CJK, emoji, combining characters, etc.
	return runewidth.RuneWidth(r)
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
