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

	// Tool display styles — light grey background for all states.
	ToolBg = lipgloss.Color("#3a3a3e")

	// Tool text styles.
	ToolTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff"))
	ToolOutput = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0e0e0"))
	ToolMeta   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ca3af"))
	BashHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true)

	// Input styles.
	BlockInputBg = lipgloss.Color("#2a2a2a")
	BlockInputFg = lipgloss.Color("#e5e7eb")

	// Message styles.
	UserPrompt  = lipgloss.NewStyle().Foreground(lipgloss.Color("#93c5fd"))
	UserCaret   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	InputCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	ErrorMessage = Red.Copy()
)

// previewLines caps output preview at this many lines when the tool call is not expanded.
const previewLines = 20

// RenderToolCall renders a tool call as a background-colored block (pi-mono style).
// No box borders — the background color indicates state:
//
//	pending (args received, not executing) → ToolPendingBg
//	running (executing)                     → ToolPendingBg
//	done, success                           → ToolSuccessBg
//	done, error                             → ToolErrorBg
func RenderToolCall(tc toolRenderItem, width int) string {
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
		preview := formatOutputPreview(tc.output, inner-2)
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

// formatOutputPreview returns a preview of the tool output, capped at previewLines.
func formatOutputPreview(output string, width int) string {
	lines := strings.Split(output, "\n")

	// Strip trailing empty lines.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	hidden := 0
	if len(lines) > previewLines {
		hidden = len(lines) - previewLines
		lines = lines[:previewLines]
	}

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ToolOutput.Render(truncateToWidth(line, width)))
	}

	if hidden > 0 {
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
