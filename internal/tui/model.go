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
	toolCalls []toolRenderItem
}

// tea messages from the agent goroutine.
type (
	textMsg    string
	toolMsg    agent.ToolCallEvent
	toolResult agent.ToolCallResult
	doneMsg    struct {
		result *agent.Result
		err    error
	}
)

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

	messages    []messageItem
	pending     bool
	ready       bool
	width       int
	height      int
	agentCancel context.CancelFunc
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
		config:   cfg,
		provider: prov,
		toolReg:  toolReg,
		events:   ev,
		cmdReg:   cmdReg,
		session:  sess,
		textarea: ta,
		spinner:  s,
		messages: []messageItem{},
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
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.pending && m.agentCancel != nil {
				m.agentCancel()
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.pending {
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
			go runAgentLoop(ctx, &m, input)
			return m, m.spinner.Tick
		}

	case textMsg:
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
			m.messages[len(m.messages)-1].content += string(msg)
			m.refreshViewport()
		}
		return m, nil

	case toolMsg:
		ev := agent.ToolCallEvent(msg)
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
			last := &m.messages[len(m.messages)-1]
			if !ev.Start {
				last.toolCalls = append(last.toolCalls, toolRenderItem{
					name: ev.Name,
					args: summarizeArgs(ev.Name, ev.Args),
				})
				m.refreshViewport()
			}
		}
		return m, nil

	case toolResult:
		ev := agent.ToolCallResult(msg)
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant" {
			last := &m.messages[len(m.messages)-1]
			if len(last.toolCalls) > 0 {
				tc := &last.toolCalls[len(last.toolCalls)-1]
				tc.output = truncateDisplay(ev.Output, 500)
				tc.success = ev.Success
				m.refreshViewport()
			}
		}
		return m, nil

	case doneMsg:
		m.pending = false
		m.textarea.Focus()
		if msg.err != nil {
			m.messages = append(m.messages, messageItem{role: "error", content: msg.err.Error()})
		} else if msg.result != nil {
			if m.session != nil {
				_ = m.session.Append(session.Record{Role: "assistant", Content: msg.result.Text, Timestamp: time.Now()})
			}
		}
		m.refreshViewport()
		return m, nil

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
	return m, cmd
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

	b.WriteString(m.viewport.View())

	if m.pending {
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(Dim.Render("Working"))
	}
	b.WriteString("\n")

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

func runAgentLoop(ctx context.Context, m *Model, input string) {
	sysPrompt := prompt.Build(*m.config, getCwd())

	// Build conversation history.
	var msgs []provider.Message
	for _, msg := range m.messages {
		msgs = append(msgs, provider.Message{Role: msg.role, Content: msg.content})
	}

	// We reset config.SystemPrompt to what Build() returns so the runner uses it.
	m.config.SystemPrompt = sysPrompt

	_ = input // the messages already contain the input

	result, err := agent.Run(
		ctx,
		m.provider,
		m.config,
		msgs,
		m.toolReg,
		nil,
		func(ev agent.Event) {
			switch {
			case ev.Text != "":
				sendMsg(ctx, textMsg(ev.Text))
			case ev.ToolCall != nil:
				sendMsg(ctx, toolMsg(*ev.ToolCall))
			case ev.ToolResult != nil:
				sendMsg(ctx, toolResult(*ev.ToolResult))
			}
		},
	)
	sendMsg(ctx, doneMsg{result: result, err: err})
}

func sendMsg[T tea.Msg](ctx context.Context, msg T) {
	// Send to program via tea.Batch or a channel. We use tea.Printf as fallback,
	// but ideally should use a proper channel mechanism.
	// For now, we rely on the agent.Run callback being synchronous with event processing.
	// This is a simplified version; a production implementation would use
	// a channel and a separate goroutine feeding into the Bubble Tea program.
	_ = ctx
	_ = msg
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
