package provider

import "context"

// Message is a chat message in the conversation.
type Message struct {
	Role             string     `json:"role"`                        // "system", "user", "assistant", "tool"
	Content          string     `json:"content"`                     // Message text or tool result
	ReasoningContent string     `json:"reasoning_content,omitempty"` // Model reasoning (e.g. DeepSeek thinking)
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`        // Assistant tool calls
	ToolCallID       string     `json:"tool_call_id,omitempty"`      // Tool result call ID
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

// Pricing stores model pricing in USD per million tokens.
type Pricing struct {
	InputPerMTok      float64 `json:"input_per_mtok"`
	OutputPerMTok     float64 `json:"output_per_mtok"`
	CacheReadPerMTok  float64 `json:"cache_read_per_mtok"`
	CacheWritePerMTok float64 `json:"cache_write_per_mtok"`
}

// Cost stores calculated request cost in USD.
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Total      float64 `json:"total"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens      int  `json:"input_tokens"`
	OutputTokens     int  `json:"output_tokens"`
	CacheReadTokens  int  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int  `json:"cache_write_tokens,omitempty"`
	Cost             Cost `json:"cost"`
	Subscription     bool `json:"subscription,omitempty"`
}

// Request is the input to a provider Send or Stream call.
type Request struct {
	Model        string
	Thinking     string
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
	Stream       bool

	// OpenRouter response caching. Cache hits return zero billed tokens and are
	// distinct from provider prompt-cache token accounting.
	Cache      bool
	CacheTTL   int
	CacheClear bool

	// Optional metadata to include in API requests.
	// Providers extract the fields they understand and ignore the rest.
	// For example, Anthropic uses "user_id" for abuse tracking and rate limiting,
	// and OpenRouter uses "title" and "referer" for app identification.
	Metadata map[string]string
}

// Response is the result of a non-streaming provider call.
type Response struct {
	Text             string
	ReasoningContent string
	ToolCalls        []ToolCall
	Usage            *Usage
}

// StreamEvent is emitted during streaming. Exactly one field is set.
type StreamEvent struct {
	TextDelta         string    // Partial text chunk
	ReasoningDelta    string    // Partial reasoning chunk
	ToolCallStart     *ToolCall // Tool call identified (name + ID known)
	ToolCallArgsDelta string    // Partial JSON args for current tool call
	ToolCallEnd       *ToolCall // Tool call complete with full parsed args
	Done              *Usage    // Stream finished with final usage
	Error             error     // Stream error
}

// ModelInfo describes a model exposed by a provider.
type ModelInfo struct {
	ID            string
	ContextLength int
	Pricing       Pricing
}

// ModelInfoProvider is optionally implemented by providers that can return model metadata.
type ModelInfoProvider interface {
	ListModelInfo(ctx context.Context) ([]ModelInfo, error)
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
