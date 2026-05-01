package main

import (
	"fmt"
	"strings"

	"crobot/internal/version"
)

// startupArgs holds parsed CLI flag values.
type startupArgs struct {
	continueRecent bool
	noSession      bool
	sessionPath    string
	help           bool
	showVersion    bool
	skillPaths     []string
	promptText     string // headless mode prompt
}

// cliFlag describes a single command-line flag — the canonical source for
// parsing, help text, and documentation. To add a new flag:
//   1. Add a field to startupArgs
//   2. Add a cliFlag entry to startupFlags
//   3. Add a handler entry to flagHandlers
type cliFlag struct {
	Name        string // e.g. "--continue"
	Short       string // e.g. "-c" (empty if none)
	Arg         string // e.g. "<path>" (empty if none)
	Description string
}

// startupFlags is the canonical list of all supported CLI flags.
var startupFlags = []cliFlag{
	{Name: "--help", Short: "-h", Description: "Show help and exit"},
	{Name: "--version", Short: "-v", Description: "Show version and exit"},
	{Name: "--continue", Short: "-c", Description: "Continue the most recent session"},
	{Name: "--session", Arg: "<path>", Description: "Open a specific session file"},
	{Name: "--no-session", Description: "Run without saving a session"},
	{Name: "--skill", Arg: "<path>", Description: "Load a skill from a directory or .md file (repeatable)"},
	{Name: "--prompt", Short: "-p", Arg: "<text>", Description: "Run in headless mode with a single prompt; print response to stdout"},
}

// flagHandler receives the current arg index, full args slice, and a pointer
// to startupArgs. It returns the number of args consumed (1 for boolean,
// 2 for flags with an argument) or 0 if the flag is unrecognized.
type flagHandler func(args []string, i int, parsed *startupArgs) (consumed int, err error)

// flagHandlers maps flag names (long and short) to their behavior.
// The parser builds this from startupFlags metadata at init time.
var flagHandlers = map[string]flagHandler{
	"--help": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.help = true
		return 1, nil
	},
	"-h": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.help = true
		return 1, nil
	},
	"help": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.help = true
		return 1, nil
	},
	"--continue": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.continueRecent = true
		return 1, nil
	},
	"-c": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.continueRecent = true
		return 1, nil
	},
	"--session": func(args []string, i int, p *startupArgs) (int, error) {
		if i+1 >= len(args) {
			return 0, fmt.Errorf("--session requires a path")
		}
		p.sessionPath = args[i+1]
		return 2, nil
	},
	"--version": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.showVersion = true
		return 1, nil
	},
	"-v": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.showVersion = true
		return 1, nil
	},
	"--no-session": func(_ []string, _ int, p *startupArgs) (int, error) {
		p.noSession = true
		return 1, nil
	},
	"--skill": func(args []string, i int, p *startupArgs) (int, error) {
		if i+1 >= len(args) {
			return 0, fmt.Errorf("--skill requires a path")
		}
		p.skillPaths = append(p.skillPaths, args[i+1])
		return 2, nil
	},
	"--prompt": func(args []string, i int, p *startupArgs) (int, error) {
		if i+1 >= len(args) {
			return 0, fmt.Errorf("--prompt requires text")
		}
		p.promptText = args[i+1]
		return 2, nil
	},
	"-p": func(args []string, i int, p *startupArgs) (int, error) {
		if i+1 >= len(args) {
			return 0, fmt.Errorf("-p requires text")
		}
		p.promptText = args[i+1]
		return 2, nil
	},
}

// parseStartupArgs parses startupFlags from args, returning parsed values
// and any remaining non-flag arguments.
func parseStartupArgs(args []string) (startupArgs, []string, error) {
	var parsed startupArgs
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		h, ok := flagHandlers[args[i]]
		if !ok {
			remaining = append(remaining, args[i])
			continue
		}
		consumed, err := h(args, i, &parsed)
		if err != nil {
			return parsed, nil, err
		}
		i += consumed - 1 // -1 because the loop adds 1
	}
	if parsed.noSession && (parsed.continueRecent || parsed.sessionPath != "") {
		return parsed, nil, fmt.Errorf("--no-session cannot be combined with --continue or --session")
	}
	return parsed, remaining, nil
}

// cliHelpText returns the CLI help output, derived from startupFlags.
func cliHelpText() string {
	var b strings.Builder
	b.WriteString("Crobot\n\n")
	b.WriteString("Usage:\n")
	b.WriteString("  crobot [flags]\n")
	b.WriteString("  crobot help\n\n")
	b.WriteString("Flags:\n")
	// Show --version first (special, has short flag -v).
	b.WriteString(fmt.Sprintf("  -v, --version          Show version and exit (version %s)\n", version.Version))
	for _, f := range startupFlags {
		if f.Name == "--version" {
			continue // shown above with version info
		}
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

// cliVersionText returns the version output for --version.
func cliVersionText() string {
	return fmt.Sprintf("crobot %s (commit %s, built %s)\n", version.Version, version.Commit, version.BuildDate)
}
