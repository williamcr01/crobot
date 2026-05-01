package tools

import (
	"context"
	"fmt"
	"strings"

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
	if t.Source == "" {
		t.Source = "native"
	}
	r.tools[t.Name] = t
}

// Has reports whether a tool is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// Get returns a registered tool.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Unregister removes a tool by name.
func (r *Registry) Unregister(name string) {
	delete(r.tools, name)
}

// UnregisterBySource removes all tools from the given source.
func (r *Registry) UnregisterBySource(source string) {
	for name, t := range r.tools {
		if t.Source == source {
			delete(r.tools, name)
		}
	}
}

// UnregisterPluginTools removes all plugin-provided tools.
func (r *Registry) UnregisterPluginTools() {
	for name, t := range r.tools {
		if strings.HasPrefix(t.Source, "plugin:") {
			delete(r.tools, name)
		}
	}
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
