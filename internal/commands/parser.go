package commands

import (
	"fmt"
	"strings"
)

// Handler is a function that handles a slash command.
// It receives parsed arguments and returns a result string or an error.
type Handler func(args []string) (string, error)

// Command describes a single slash command.
type Command struct {
	Name        string
	Description string
	Args        string // usage hint, e.g. "<model>"
	Handler     Handler
}

// Registry manages available slash commands.
type Registry struct {
	commands map[string]Command
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]Command)}
}

// Register adds a command to the registry.
func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name] = cmd
}

// Execute parses input and runs the matching command.
// If input does not start with "/", ok is false and no error is returned.
func (r *Registry) Execute(input string) (string, error) {
	cmdName, args, ok := Parse(input)
	if !ok {
		return "", nil
	}

	cmd, exists := r.commands[cmdName]
	if !exists {
		return "", fmt.Errorf("unknown command: /%s", cmdName)
	}

	return cmd.Handler(args)
}

// HelpText returns a formatted list of all registered commands.
func (r *Registry) HelpText() string {
	var b strings.Builder
	b.WriteString("Commands:\n")
	for _, cmd := range r.commands {
		line := fmt.Sprintf("  /%s", cmd.Name)
		if cmd.Args != "" {
			line += " " + cmd.Args
		}
		if cmd.Description != "" {
			line += "  " + cmd.Description
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// Parse splits an input line into command name and args.
// Returns false if the input doesn't start with "/".
func Parse(input string) (cmd string, args []string, ok bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || trimmed[0] != '/' {
		return "", nil, false
	}

	// Strip leading "/".
	trimmed = trimmed[1:]
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "", nil, false
	}

	cmd = parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}
	return cmd, args, true
}
