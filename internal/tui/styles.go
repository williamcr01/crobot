package tui

import (
	"fmt"
	"strings"

	"crobot/internal/themes"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds all Lipgloss styles for the TUI, built from a theme.
type Styles struct {
	Dim          lipgloss.Style
	Bold         lipgloss.Style
	Cyan         lipgloss.Style
	Green        lipgloss.Style
	Yellow       lipgloss.Style
	Red          lipgloss.Style
	Gray         lipgloss.Style
	ToolBg       lipgloss.Color // raw color used for block backgrounds
	ToolTitle    lipgloss.Style
	ToolOutput   lipgloss.Style
	ToolMeta     lipgloss.Style
	BashHeader   lipgloss.Style
	UserPrompt   lipgloss.Style
	UserCaret    lipgloss.Style
	InputCursor  lipgloss.Style
	ErrorMessage lipgloss.Style

	// Markdown heading styles.
	H1Style lipgloss.Style
	H2Style lipgloss.Style
	H3Style lipgloss.Style
	H4Style lipgloss.Style

	// Markdown inline styles.
	BoldStyle   lipgloss.Style
	ItalicStyle lipgloss.Style
	CodeStyle   lipgloss.Style
	StrikeStyle lipgloss.Style
	LinkStyle   lipgloss.Style
	ImageStyle  lipgloss.Style

	// Markdown block styles.
	BodyTextStyle  lipgloss.Style
	ThinkingStyle  lipgloss.Style
	CodeBlockStyle lipgloss.Style
	QuoteStyle     lipgloss.Style
	QuoteBar       lipgloss.Style
	HRStyle        lipgloss.Style

	// Markdown task list styles.
	TaskDoneStyle lipgloss.Style
	TaskOpenStyle lipgloss.Style

	// Markdown table styles.
	TableBorder lipgloss.Style
	TableHeader lipgloss.Style
	TableCell   lipgloss.Style
}

// NewStyles builds a Styles struct from a theme.
func NewStyles(t *themes.Theme) Styles {
	c := t.Colors
	b := t.Bold

	return Styles{
		Dim:            fg(c[themes.StyleDim]),
		Bold:           bold(b[themes.StyleBold]),
		Cyan:           fg(c[themes.StyleCyan]),
		Green:          fg(c[themes.StyleGreen]),
		Yellow:         fg(c[themes.StyleYellow]),
		Red:            fg(c[themes.StyleRed]),
		Gray:           fg(c[themes.StyleGray]),
		ToolBg:         lipgloss.Color(c[themes.StyleToolBg]),
		ToolTitle:      fg(c[themes.StyleToolTitle]).Bold(b[themes.StyleToolTitle]),
		ToolOutput:     fg(c[themes.StyleToolOutput]),
		ToolMeta:       fg(c[themes.StyleToolMeta]),
		BashHeader:     fg(c[themes.StyleBashHeader]).Bold(b[themes.StyleBashHeader]),
		UserPrompt:     fg(c[themes.StyleUserPrompt]),
		UserCaret:      fg(c[themes.StyleUserCaret]),
		InputCursor:    fg(c[themes.StyleInputCursor]),
		ErrorMessage:   fg(c[themes.StyleErrorMessage]),
		H1Style:        fg(c[themes.StyleH1]).Bold(b[themes.StyleH1]),
		H2Style:        fg(c[themes.StyleH2]).Bold(b[themes.StyleH2]),
		H3Style:        fg(c[themes.StyleH3]).Bold(b[themes.StyleH3]),
		H4Style:        fg(c[themes.StyleH4]).Bold(b[themes.StyleH4]),
		BoldStyle:      lipgloss.NewStyle().Bold(true),
		ItalicStyle:    lipgloss.NewStyle().Italic(true),
		CodeStyle:      fg(c[themes.StyleCode]),
		StrikeStyle:    fg(c[themes.StyleStrike]).Strikethrough(true),
		LinkStyle:      fg(c[themes.StyleLink]).Underline(true),
		ImageStyle:     fg(c[themes.StyleImage]),
		BodyTextStyle:  fg(c[themes.StyleBodyText]),
		ThinkingStyle:  fg(c[themes.StyleThinking]),
		CodeBlockStyle: fg(c[themes.StyleCodeBlock]).Background(lipgloss.Color(c[themes.StyleToolBg])),
		QuoteStyle:     fg(c[themes.StyleQuote]),
		QuoteBar:       fg(c[themes.StyleQuoteBar]),
		HRStyle:        fg(c[themes.StyleHR]),
		TaskDoneStyle:  fg(c[themes.StyleTaskDone]),
		TaskOpenStyle:  fg(c[themes.StyleTaskOpen]),
		TableBorder:    fg(c[themes.StyleTableBorder]),
		TableHeader:    fg(c[themes.StyleTableHeader]).Bold(b[themes.StyleTableHeader]),
		TableCell:      fg(c[themes.StyleTableCell]),
	}
}

// style helpers
func fg(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

func bold(b bool) lipgloss.Style {
	return lipgloss.NewStyle().Bold(b)
}

// collapsedPreviewLines caps output preview at this many lines when collapsed.
const collapsedPreviewLines = 10

// RenderToolCall renders a tool call as a background-colored block (pi-mono style).
// No box borders. When expanded is false and output exceeds collapsedPreviewLines,
// only a preview is shown with a "ctrl+o to expand" hint.
// WebTools are tools whose output should be rendered as markdown.
var WebTools = map[string]bool{
	"web_search":          true,
	"web_fetch":           true,
	"get_search_content":  true,
}

// ReadOnlyTools are tools whose output is hidden from the user TUI display.
// The call line (name + args) is still shown, but the result content is not rendered.
var ReadOnlyTools = map[string]bool{
	"file_read": true,
	"grep":      true,
	"find":      true,
	"ls":        true,
}

func RenderToolCall(tc toolRenderItem, width int, expanded bool, s Styles) string {
	inner := width - 4
	if inner < 20 {
		inner = 20
	}

	bgColor := s.ToolBg
	blockStyles := s
	blockStyles.ToolTitle = blockStyles.ToolTitle.Background(bgColor)
	blockStyles.ToolOutput = blockStyles.ToolOutput.Background(bgColor)
	blockStyles.ToolMeta = blockStyles.ToolMeta.Background(bgColor)
	blockStyles.BashHeader = blockStyles.BashHeader.Background(bgColor)
	blockStyles.Green = blockStyles.Green.Background(bgColor)
	blockStyles.Red = blockStyles.Red.Background(bgColor)
	blockStyles.Cyan = blockStyles.Cyan.Background(bgColor)
	blockStyles.Dim = blockStyles.Dim.Background(bgColor)

	statusIcon := "…"
	statusColor := blockStyles.ToolMeta

	switch tc.state {
	case toolRunning:
		statusIcon = "⏳"
	case toolDone:
		if tc.success {
			statusIcon = "✓"
			statusColor = blockStyles.Green
		} else {
			statusIcon = "✗"
			statusColor = blockStyles.Red
		}
	}

	block := lipgloss.NewStyle().
		Background(bgColor).
		Width(inner).
		Padding(0, 1)

	// Build the full tool block content: status icon + call line + optional output.
	var content strings.Builder

	// --- Status icon before call line (only shown on completion) ---
	if tc.state == toolDone {
		content.WriteString(statusColor.Render(statusIcon))
		content.WriteString(" ")
	}

	// --- Call line ---
	callLine := formatSingleToolCallLine(tc, blockStyles)
	callLine = truncateToWidth(callLine, inner-2)
	content.WriteString(callLine)

	// --- Output preview (hidden for read-only tools) ---
	if tc.output != "" && !ReadOnlyTools[tc.name] {
		content.WriteString("\n")
		collapsed := !expanded
		maxLines := 1<<31 - 1 // effectively unlimited when expanded
		if collapsed {
			maxLines = collapsedPreviewLines
		}

		if WebTools[tc.name] {
			// Render web tool output through the markdown renderer.
			mdWidth := inner - 4
			if mdWidth < 20 {
				mdWidth = 20
			}
			md := RenderMarkdown(tc.output, mdWidth, s)
			preview := formatOutputPreview(md, inner-2, maxLines, collapsed, blockStyles)
			content.WriteString(preview)
		} else if isDiffOutput(tc.name, tc.output) {
			preview := formatDiffPreview(tc.output, inner-2, maxLines, collapsed, blockStyles)
			content.WriteString(preview)
		} else {
			preview := formatOutputPreview(tc.output, inner-2, maxLines, collapsed, blockStyles)
			content.WriteString(preview)
		}
	}

	// Render the entire content as a single block for uniform background.
	return block.Render(content.String())
}

// formatSingleToolCallLine formats the call line for a single tool call.
func formatSingleToolCallLine(tc toolRenderItem, s Styles) string {
	label := s.ToolTitle.Render(tc.name)

	if tc.args == "" {
		return label
	}

	// For bash, use green formatted header like pi-mono: $ command
	if tc.name == "bash" {
		return s.BashHeader.Render(tc.args)
	}

	return label + s.ToolOutput.Render(" ") + s.ToolOutput.Render(tc.args)
}

// formatOutputPreview returns a preview of the tool output, capped at maxLines.
func formatOutputPreview(output string, width int, maxLines int, collapsed bool, s Styles) string {
	lines := strings.Split(output, "\n")

	// Strip trailing empty lines.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	hidden := 0
	if len(lines) > maxLines {
		hidden = len(lines) - maxLines
		lines = lines[:maxLines]
	}

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s.ToolOutput.Render(truncateToWidth(line, width)))
	}

	if collapsed && hidden > 0 {
		b.WriteByte('\n')
		b.WriteString(s.ToolMeta.Render(fmt.Sprintf("… %d more lines (ctrl+o to expand)", hidden)))
	} else if hidden > 0 {
		b.WriteByte('\n')
		b.WriteString(s.ToolMeta.Render(fmt.Sprintf("… %d more lines", hidden)))
	}

	return b.String()
}

// truncateToWidth truncates a string to fit within a given visible width, appending "…" if needed.
// It is ANSI-aware — uses lipgloss.Width to measure visible column width.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	// Manually truncate while preserving ANSI escape sequences.
	var b strings.Builder
	visible := 0
	runes := []rune(s)
	for i := 0; i < len(runes) && visible < maxWidth-1; i++ {
		r := runes[i]
		if r == '\x1b' {
			// Copy the full ANSI escape sequence: \x1b[...letter
			b.WriteRune(r)
			i++
			if i < len(runes) && runes[i] == '[' {
				b.WriteRune(runes[i])
				for i+1 < len(runes) {
					next := runes[i+1]
					if next >= 'A' && next <= 'Z' || next >= 'a' && next <= 'z' {
						i++
						b.WriteRune(runes[i])
						break
					}
					i++
					b.WriteRune(runes[i])
				}
			}
			continue
		}
		b.WriteRune(r)
		visible++
	}
	b.WriteRune('…')
	return b.String()
}

func fmtSprintf(format string, a ...interface{}) string {
	return fmt.Sprintf(format, a...)
}
