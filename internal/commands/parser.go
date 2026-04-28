package commands

import (
	"fmt"
	"sort"
	"strings"
)

// ModelInfo represents a model from a provider.
type ModelInfo struct {
	ID       string
	Provider string
}

// ModelRegistry interface for listing/filtering models.
type ModelRegistry interface {
	GetAll() []ModelInfo
	Filter(prefix string) []ModelInfo
}

// Handler is a function that handles a slash command.
// It receives parsed arguments and returns a result string or an error.
type Handler func(args []string) (string, error)

// Command describes a single slash command.
type Command struct {
	Name        string
	Description string
	Args        string // usage hint, e.g. "<model>"
	Handler     Handler
	ModelID     string // non-empty if this is a model suggestion (e.g., "openrouter/anthropic/claude-opus-4-7")
}

// Registry manages available slash commands.
type Registry struct {
	commands     map[string]Command
	modelRegistry ModelRegistry
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]Command)}
}

// SetModelRegistry sets the model registry for /model suggestions.
func (r *Registry) SetModelRegistry(mr ModelRegistry) {
	r.modelRegistry = mr
}

// FilterModels returns model suggestions matching the filter text.
func (r *Registry) FilterModels(filter string) []Command {
	if r.modelRegistry == nil {
		return nil
	}
	models := r.modelRegistry.Filter(filter)
	var suggestions []Command
	for _, m := range models {
		suggestions = append(suggestions, Command{
			Name:    m.ID,
			Args:    m.Provider,
			ModelID: m.ID,
		})
	}
	return suggestions
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
	for _, cmd := range r.Commands() {
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

// Commands returns all registered commands sorted by name.
func (r *Registry) Commands() []Command {
	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// Suggestions returns commands matching the slash-command prefix currently being typed.
func (r *Registry) Suggestions(input string) []Command {
	trimmed := strings.TrimLeft(input, " \t")
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}

	withoutSlash := strings.TrimPrefix(trimmed, "/")

	// Hide suggestions once args are typed (space present).
	if strings.ContainsAny(withoutSlash, " \t\n") {
		return nil
	}

	prefix := withoutSlash
	return r.commandSuggestions(prefix)
}

// commandSuggestions returns commands matching the prefix.
func (r *Registry) commandSuggestions(prefix string) []Command {
	matches := make([]Command, 0)
	for _, cmd := range r.Commands() {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// modelSuggestions returns model suggestions for /model context.
func (r *Registry) modelSuggestions(input string) []Command {
	if r.modelRegistry == nil {
		return nil
	}

	// Extract filter text after "model" or "model "
	var prefix string
	if len(input) > 5 && input[5] == ' ' {
		prefix = input[6:] // after "model "
	}
	// If just "model" without space, prefix stays empty (show all)

	models := r.modelRegistry.Filter(prefix)
	var suggestions []Command
	for _, m := range models {
		suggestions = append(suggestions, Command{
			Name:    m.ID,
			Args:    m.Provider,
			ModelID: m.ID,
		})
	}
	return suggestions
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
