package provider

import "context"

// Message is a chat message in the conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant", "tool"
	Content string `json:"content"` // Message text or tool result
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"` // JSON Schema object
}

// ToolCall represents a function_call from the model.
type ToolCall struct {
	Name string         `json:"name"`
	ID   string         `json:"id"`
	Args map[string]any `json:"args"`
}

// ToolResult is the output of an executed tool.
type ToolResult struct {
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Request is the input to a provider Send or Stream call.
type Request struct {
	Model        string
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
	Stream       bool
}

// Response is the result of a non-streaming provider call.
type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     *Usage
}

// StreamEvent is emitted during streaming. Exactly one field is set.
type StreamEvent struct {
	TextDelta        string     // Partial text chunk
	ReasoningDelta   string     // Partial reasoning chunk
	ToolCallStart    *ToolCall  // Tool call identified (name + ID known)
	ToolCallArgsDelta string    // Partial JSON args for current tool call
	ToolCallEnd      *ToolCall  // Tool call complete with full parsed args
	Done             *Usage     // Stream finished with final usage
	Error            error      // Stream error
}

// Provider abstracts an LLM backend.
type Provider interface {
	// Name returns a human-readable provider identifier (e.g. "openrouter").
	Name() string

	// Send performs a non-streaming completion.
	Send(ctx context.Context, req Request) (*Response, error)

	// Stream performs a streaming completion. The returned channel is closed
	// when streaming completes or fails.
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)

	// ListModels returns all available model IDs.
	ListModels(ctx context.Context) ([]string, error)
}

// Factory creates a Provider from an API key.
type Factory func(apiKey string) (Provider, error)

var registry = map[string]Factory{}

// Register adds a provider factory to the global registry.
func Register(name string, factory Factory) {
	registry[name] = factory
}

// Create instantiates a provider by name with the given API key.
func Create(name, apiKey string) (Provider, error) {
	f, ok := registry[name]
	if !ok {
		return nil, ErrUnsupportedProvider(name)
	}
	return f(apiKey)
}
