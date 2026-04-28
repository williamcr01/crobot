package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"crobot/internal/agent"
	"crobot/internal/commands"
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

// toolRenderItem holds rendered state for one tool call in the view.
type toolRenderItem struct {
	name     string
	args     string
	output   string
	success  bool
	duration time.Duration
}

// messageItem holds one rendered message in the conversation view.
type messageItem struct {
	role      string // "user", "assistant", "system", "error"
	content   string
	usage     string
	toolCalls []toolRenderItem
}

// tea messages from the agent goroutine.
type agentEventMsg agent.Event

// sessionWriter is the subset of session.Manager used by the TUI.
type sessionWriter interface {
	Append(rec session.Record) error
	Path() string
}

// Model is the root Bubble Tea model for the agent TUI.
type Model struct {
	config   *config.AgentConfig
	provider provider.Provider
	toolReg  *tools.Registry
	session  sessionWriter
	events   *events.Logger
	cmdReg   *commands.Registry

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
}

// NewModel creates a fully initialized TUI model.
func NewModel(
	cfg *config.AgentConfig,
	prov provider.Provider,
	toolReg *tools.Registry,
	ev *events.Logger,
	cmdReg *commands.Registry,
	sess sessionWriter,
) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true)

	s := NewLoaderSpinner()

	return &Model{
		config:      cfg,
		provider:    prov,
		toolReg:     toolReg,
		events:      ev,
		cmdReg:      cmdReg,
		session:     sess,
		textarea:    ta,
		spinner:     s,
		messages:    []messageItem{},
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

		headerHeight := 0
		if m.config.ShowBanner {
			headerHeight = 9
		}
		footerHeight := 4
		if m.pending {
			footerHeight = 5
		}
		vpHeight := msg.Height - headerHeight - footerHeight
		if vpHeight < 10 {
			vpHeight = 10
		}
		m.viewport = viewport.New(msg.Width, vpHeight)
		m.viewport.YPosition = 0
		m.viewport.SetContent(m.renderMessages())

		m.textarea.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.pending && m.agentCancel != nil {
				m.agentCancel()
			}
			return m, tea.Quit

		case tea.KeyEsc:
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
			}

		case tea.KeyEnter:
			if m.pending {
				return m, nil
			}
			if suggestions := m.commandSuggestions(); len(suggestions) > 0 && !m.commandInputExactlyMatchesSuggestion(suggestions) {
				m.completeCommandSuggestion(suggestions)
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
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
					m.messages = append(m.messages, messageItem{role: "system", content: result})
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

	case agentEventMsg:
		return m.handleAgentEvent(agent.Event(msg))

	case spinner.TickMsg:
		if m.pending {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.clampCommandSuggestionIndex()
	return m, cmd
}

// waitForEvents returns a tea.Cmd that reads from the agentEvents channel.
func (m Model) waitForEvents() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.agentEvents
		if !ok {
			return nil
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

	case "tool_call_start":
		// Tool calls are handled by tool_call_end.

	case "tool_call_end":
		if ev.ToolCallEnd != nil {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				last.toolCalls = append(last.toolCalls, toolRenderItem{
					name: ev.ToolCallEnd.Name,
					args: summarizeArgs(ev.ToolCallEnd.Name, ev.ToolCallEnd.Args),
				})
				m.refreshViewport()
			}
		}

	case "tool_exec_result":
		if ev.ToolExecResult != nil {
			ter := ev.ToolExecResult
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
				last := &m.messages[len(m.messages)-1]
				if len(last.toolCalls) > 0 {
					tc := &last.toolCalls[len(last.toolCalls)-1]
					tc.output = truncateDisplay(ter.Output, 500)
					tc.success = ter.Success
					tc.duration = time.Duration(ter.Duration) * time.Millisecond
					m.refreshViewport()
				}
			}
		}

	case "turn_end", "message_end":
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

	if m.config.ShowBanner {
		b.WriteString(Render(m.config.Model))
		b.WriteString("\n")
	}

	viewport := m.viewport
	if m.height > 0 {
		viewport.Height = m.dynamicViewportHeight()
	}
	b.WriteString(viewport.View())

	if m.pending {
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(Dim.Render("Working"))
	}
	b.WriteString("\n")

	if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
		b.WriteString(m.renderCommandSuggestions(suggestions))
		b.WriteString("\n")
	}

	switch m.config.Display.InputStyle {
	case "block":
		b.WriteString(renderBlockInput(m.width, m.textarea.Value()))
	case "bordered":
		b.WriteString(renderBorderedInput(m.width, m.textarea.Value()))
	default:
		b.WriteString(UserCaret.Render("> ") + m.textarea.Value())
	}
	b.WriteString("\n")
	b.WriteString(Dim.Render(compactCwd()))

	return b.String()
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
	m.textarea.SetValue("/" + suggestions[m.commandSuggestionIndex].Name + " ")
	m.commandSuggestionIndex = 0
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
	start, end, selected := m.visibleCommandSuggestionRange(suggestions)

	var b strings.Builder
	b.WriteString(Dim.Render("commands"))
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
		line := fmt.Sprintf("%s/%s", prefix, cmd.Name)
		if cmd.Args != "" {
			line += " " + cmd.Args
		}
		if cmd.Description != "" {
			line += "  " + cmd.Description
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

func (m Model) dynamicViewportHeight() int {
	headerHeight := 0
	if m.config.ShowBanner {
		headerHeight = 9
	}
	footerHeight := 4 + m.commandSuggestionHeight()
	if m.pending {
		footerHeight++
	}
	vpHeight := m.height - headerHeight - footerHeight
	if vpHeight < 3 {
		vpHeight = 3
	}
	return vpHeight
}

func (m Model) renderMessages() string {
	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.role {
		case "user":
			b.WriteString("  ")
			b.WriteString(UserCaret.Render(">"))
			b.WriteString(" ")
			b.WriteString(UserPrompt.Render(msg.content))
			b.WriteString("\n\n")
		case "assistant":
			if msg.content != "" {
				b.WriteString(msg.content)
				b.WriteString("\n")
			}
			for _, tc := range msg.toolCalls {
				b.WriteString(RenderToolBox(tc.name, tc.args, tc.output, tc.duration.Milliseconds(), tc.success, m.width-4))
				b.WriteString("\n")
			}
			if msg.usage != "" {
				b.WriteString(Gray.Render(msg.usage))
				b.WriteString("\n")
			}
		case "system":
			b.WriteString(Dim.Render(msg.content))
			b.WriteString("\n\n")
		case "error":
			b.WriteString(Red.Render("Error: " + msg.content))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

// --- Agent runner goroutine ---

func (m *Model) startAgent(ctx context.Context, input string) {
	// Capture the channel locally so this goroutine only ever touches
	// its own instance, even if m.agentEvents is reassigned later.
	ch := m.agentEvents
	defer close(ch)

	sysPrompt := prompt.Build(*m.config, getCwd())

	// Build conversation history for the LLM.
	var llmMsgs []provider.Message
	for _, msg := range m.messages {
		if msg.role == "user" || msg.role == "assistant" || msg.role == "system" {
			llmMsgs = append(llmMsgs, provider.Message{Role: msg.role, Content: msg.content})
		}
	}

	// Add tool result messages from previous turns.
	for _, msg := range m.messages {
		if msg.role == "assistant" {
			for _, tc := range msg.toolCalls {
				if tc.output != "" {
					llmMsgs = append(llmMsgs, provider.Message{
						Role:    "tool",
						Content: tc.output,
					})
				}
			}
		}
	}

	// The latest user message is already in m.messages.
	// Run the agent loop.
	_, _ = agent.Run(
		ctx,
		m.provider,
		m.config.Model,
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
	result, err := reg.Execute(context.Background(), "shell", map[string]any{"command": cmd})
	if err != nil {
		return fmt.Sprintf("shell error: %v", err)
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

func renderBlockInput(width int, val string) string {
	w := width - 4
	if w < 20 {
		w = 20
	}
	return lipgloss.NewStyle().
		Background(BlockInputBg).
		Padding(0, 1).
		Width(w).
		Render("> " + val)
}

func renderBorderedInput(width int, val string) string {
	w := width - 4
	if w < 20 {
		w = 20
	}
	line := strings.Repeat("─", w)
	return fmt.Sprintf("%s\n> %s\n%s", Dim.Render(line), val, Dim.Render(line))
}

// --- Helpers ---

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

func summarizeArgs(name string, args map[string]any) string {
	if args == nil {
		return ""
	}
	key := summarizeKey(name)
	if v, ok := args[key]; ok {
		val := fmt.Sprintf("%v", v)
		if len(val) > 40 {
			val = val[:40] + "..."
		}
		return key + "=" + val
	}
	return ""
}

func summarizeKey(name string) string {
	switch name {
	case "shell":
		return "command"
	case "file_read", "file_write", "file_edit":
		return "path"
	default:
		return ""
	}
}
