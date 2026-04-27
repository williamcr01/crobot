package tui

import (
	"fmt"
	"strings"

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

	// Tool display styles.
	ToolLabel  = Yellow.Copy().Bold(true)
	ToolBranch = Dim.Copy()
	ToolOutput = Dim.Copy()
	ToolSuccess = Green.Copy()

	// Input styles.
	BlockInputBg = lipgloss.Color("#2a2a2a")
	BlockInputFg = lipgloss.Color("#e5e7eb")

	// Message styles.
	UserPrompt  = lipgloss.NewStyle().Foreground(lipgloss.Color("#93c5fd"))
	UserCaret   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	ErrorMessage = Red.Copy()
)

func RenderToolBox(name, args, output string, durationMs int64, success bool, width int) string {
	inner := width - 4
	if inner < 20 {
		inner = 20
	}

	// Header line.
	label := ToolLabel.Render(name)
	summary := Dim.Render(args)
	header := "╭─ " + label
	if args != "" {
		header += " " + summary
	}
	header += " " + strings.Repeat("─", inner-len([]rune(header[2:]))) + "╮"

	// Body lines.
	var bodyLines []string
	if output != "" {
		truncated := output
		truncLines := strings.Split(truncated, "\n")
		if len(truncLines) > 10 {
			truncLines = truncLines[:10]
			truncLines = append(truncLines, "...")
		}
		for i, line := range truncLines {
			// Indent and wrap.
			styled := ToolOutput.Render("  " + line)
			if len(line) > inner-2 {
				styled = ToolOutput.Render("  " + line[:inner-6] + "...")
			}
			if i < len(truncLines)-1 {
				bodyLines = append(bodyLines, "│"+styled+strings.Repeat(" ", inner-len([]rune(line))-2)+"│")
			} else {
				bodyLines = append(bodyLines, "│"+styled+strings.Repeat(" ", inner-len([]rune(line))-2)+"│")
			}
		}
	}

	// Footer line.
	duration := Dim.Render(fmt.Sprintf("%.1fs", float64(durationMs)/1000))
	check := ToolSuccess.Render("ok")
	if !success {
		check = Red.Render("err")
	}
	footer := "╰─ " + duration + " " + check + " " + strings.Repeat("─", inner-len([]rune(duration+check))-3) + "╯"

	return strings.Join(append([]string{header}, append(bodyLines, footer)...), "\n")
}

func fmtSprintf(format string, a ...interface{}) string {
	return fmt.Sprintf(format, a...)
}