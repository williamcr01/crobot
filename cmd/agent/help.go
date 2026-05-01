package main

import (
	"fmt"
	"strings"
)

// cliFlag describes a single command-line flag.
type cliFlag struct {
	Name        string // e.g. "--continue"
	Short       string // e.g. "-c" (empty if none)
	Arg         string // e.g. "<path>" (empty if none)
	Description string
}

// startupFlags lists all supported CLI flags.
var startupFlags = []cliFlag{
	{Name: "--help", Short: "-h", Description: "Show help and exit"},
	{Name: "--continue", Short: "-c", Description: "Continue the most recent session"},
	{Name: "--session", Arg: "<path>", Description: "Open a specific session file"},
	{Name: "--no-session", Description: "Run without saving a session"},
}

func cliHelpText() string {
	var b strings.Builder
	b.WriteString("Crobot\n\n")
	b.WriteString("Usage:\n")
	b.WriteString("  crobot [flags]\n")
	b.WriteString("  crobot help\n\n")
	b.WriteString("Flags:\n")
	for _, f := range startupFlags {
		line := "  "
		if f.Short != "" {
			line += fmt.Sprintf("%s, %s", f.Short, f.Name)
		} else {
			line += fmt.Sprintf("    %s", f.Name)
		}
		if f.Arg != "" {
			line += " " + f.Arg
		}
		line += "  " + f.Description
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\nInside the TUI:\n")
	b.WriteString("  Type /help to show slash commands.\n")
	return b.String()
}
