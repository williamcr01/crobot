package tools

import (
	"context"
	"fmt"

	"crobot/internal/provider"
)

// Tool is a function-callable tool definition with its execution handler.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Execute     func(ctx context.Context, args map[string]any) (any, error)
	Source      string // "native" | "plugin:<name>"
}

// Registry manages tool registration and execution.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Replaces any existing tool with the same name.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name] = t
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	var out []Tool
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Execute runs the named tool with the given arguments.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (any, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, args)
}

// ToProviderTools converts the registry's tools to provider.ToolDefinition.
func (r *Registry) ToProviderTools() []provider.ToolDefinition {
	var out []provider.ToolDefinition
	for _, t := range r.tools {
		out = append(out, provider.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return out
}
