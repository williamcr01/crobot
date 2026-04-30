package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Color palette.
	Dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	Bold   = lipgloss.NewStyle().Bold(true)
	Cyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee"))
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	Gray   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af"))

	// Tool display styles — dark charcoal grey background matching code blocks.
	ToolBg = lipgloss.Color("#222222")

	// Tool text styles.
	ToolTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff"))
	ToolOutput = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af"))
	ToolMeta   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af"))
	BashHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true)

	// Message styles.
	UserPrompt  = lipgloss.NewStyle().Foreground(lipgloss.Color("#93c5fd"))
	UserCaret   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	InputCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	ErrorMessage = Red.Copy()

	// Markdown heading styles.
	H1Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))   // cyan
	H2Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e5e7eb"))   // white
	H3Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#d1d5db"))   // light gray
	H4Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ca3af"))   // dim

	// Markdown inline styles.
	BoldStyle    = lipgloss.NewStyle().Bold(true)
	ItalicStyle  = lipgloss.NewStyle().Italic(true)
	CodeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b"))                                 // amber
	StrikeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Strikethrough(true)              // dim strikethrough
	LinkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#93c5fd")).Underline(true)                  // blue underlined
	ImageStyle   = Dim.Copy()

	// Markdown block styles.
	BodyTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6d3d1")) // warm off-white for body text
	ThinkingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#78716c"))  // warm gray for reasoning
	CodeBlockStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af")).
			Background(lipgloss.Color("#222222"))
	QuoteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af"))
	QuoteBar   = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	HRStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))

	// Markdown task list styles.
	TaskDoneStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")) // green
	TaskOpenStyle = Dim.Copy()

	// Markdown table styles.
	TableBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	TableHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e5e7eb"))
	TableCell    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d1d5db"))
)

// collapsedPreviewLines caps output preview at this many lines when collapsed.
const collapsedPreviewLines = 10

// RenderToolCall renders a tool call as a background-colored block (pi-mono style).
// No box borders. When expanded is false and output exceeds collapsedPreviewLines,
// only a preview is shown with a "ctrl+o to expand" hint.
func RenderToolCall(tc toolRenderItem, width int, expanded bool) string {
	inner := width - 4
	if inner < 20 {
		inner = 20
	}

	bgColor := ToolBg
	statusIcon := "…"
	statusColor := ToolMeta

	switch tc.state {
	case toolRunning:
		statusIcon = "⏳"
	case toolDone:
		if tc.success {
			statusIcon = "✓"
			statusColor = Green.Copy()
		} else {
			statusIcon = "✗"
			statusColor = Red.Copy()
		}
	}

	block := lipgloss.NewStyle().
		Background(bgColor).
		Width(inner).
		Padding(0, 1)

	// --- Call line ---
	callLine := formatSingleToolCallLine(tc)
	callLine = truncateToWidth(callLine, inner-2)

	// --- Status line ---
	var statusLine string
	switch tc.state {
	case toolRunning:
		statusLine = ToolMeta.Render("running…")
	case toolDone:
		dur := formatDuration(tc.duration)
		statusLine = fmt.Sprintf("%s %s %s",
			statusColor.Render(statusIcon),
			ToolMeta.Render(dur),
			statusColor.Render(statusLabel(tc.success)),
		)
	default:
		// pending — no status line yet.
	}

	var b strings.Builder
	b.WriteString(block.Render(callLine))

	// Output preview.
	if tc.output != "" {
		b.WriteString("\n")
		collapsed := !expanded
		maxLines := 1<<31 - 1 // effectively unlimited when expanded
		if collapsed {
			maxLines = collapsedPreviewLines
		}
		preview := formatOutputPreview(tc.output, inner-2, maxLines, collapsed)
		outputBlock := lipgloss.NewStyle().
			Background(bgColor).
			Width(inner).
			Padding(0, 1)
		b.WriteString(outputBlock.Render(preview))
	}

	// Status footer.
	if statusLine != "" || (tc.output != "" && tc.state == toolDone) {
		b.WriteString("\n")
		statusBlock := lipgloss.NewStyle().
			Background(bgColor).
			Width(inner).
			Padding(0, 1)
		if statusLine == "" && tc.output != "" {
			statusLine = statusColor.Render(statusIcon) + " " + ToolMeta.Render(formatDuration(tc.duration))
		}
		b.WriteString(statusBlock.Render(statusLine))
	}

	return b.String()
}

// formatSingleToolCallLine formats the call line for a single tool call.
func formatSingleToolCallLine(tc toolRenderItem) string {
	label := ToolTitle.Render(tc.name)

	if tc.args == "" {
		return label
	}

	// For bash, use green formatted header like pi-mono: $ command
	if tc.name == "bash" {
		return BashHeader.Render(tc.args)
	}

	return label + " " + ToolOutput.Render(tc.args)
}

// formatOutputPreview returns a preview of the tool output, capped at maxLines.
func formatOutputPreview(output string, width int, maxLines int, collapsed bool) string {
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
		b.WriteString(ToolOutput.Render(truncateToWidth(line, width)))
	}

	if collapsed && hidden > 0 {
		b.WriteByte('\n')
		b.WriteString(ToolMeta.Render(fmt.Sprintf("… %d more lines (ctrl+o to expand)", hidden)))
	} else if hidden > 0 {
		b.WriteByte('\n')
		b.WriteString(ToolMeta.Render(fmt.Sprintf("… %d more lines", hidden)))
	}

	return b.String()
}

// formatDuration formats a duration in milliseconds for display, e.g. "1.2s" or "45ms".
func formatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms == 0 {
		return "0ms"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	sec := float64(ms) / 1000
	// Round to one decimal place.
	rounded := math.Round(sec*10) / 10
	return fmt.Sprintf("%.1fs", rounded)
}

func statusLabel(success bool) string {
	if success {
		return "ok"
	}
	return "err"
}

// truncateToWidth truncates a string to fit within a given width, appending "…" if needed.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "…"
}

func fmtSprintf(format string, a ...interface{}) string {
	return fmt.Sprintf(format, a...)
}
