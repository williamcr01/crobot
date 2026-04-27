package tools

import (
	"context"
	"fmt"

	"github.com/OpenRouterTeam/go-sdk/models/components"

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

// ToChatFunctionTools converts the registry's tools to OpenRouter SDK ChatFunctionTool values.
// Plugin-backed tools are included alongside native tools.
func (r *Registry) ToChatFunctionTools() []components.ChatFunctionTool {
	// Note: The OpenRouter SDK defines ChatFunctionTool as a union type.
	// Regular function tools use ChatFunctionToolFunction wrapper.
	// Server tools (web_search, datetime) are handled in the provider layer.
	var out []components.ChatFunctionTool
	for _, t := range r.tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}
		}
		out = append(out, components.CreateChatFunctionToolChatFunctionToolFunction(
			components.ChatFunctionToolFunction{
				Function: components.ChatFunctionToolFunctionFunction{
					Name:        t.Name,
					Description: &t.Description,
					Parameters:  params,
				},
				Type: components.ChatFunctionToolTypeFunction,
			},
		))
	}
	return out
}