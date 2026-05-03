package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

func init() {
	Register("gemini", NewGemini)
}

// GeminiProvider implements Provider using Google's Gemini API via the
// google.golang.org/genai SDK.
type GeminiProvider struct {
	name   string
	apiKey string
	client *genai.Client
}

func NewGemini(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("gemini: missing API key")
	}
	return &GeminiProvider{
		name:   "gemini",
		apiKey: apiKey,
	}, nil
}

// initClient creates the genai client lazily so constructor signature matches
// the Provider Factory.
func (p *GeminiProvider) initClient(ctx context.Context) (*genai.Client, error) {
	if p.client != nil {
		return p.client, nil
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  p.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}
	p.client = client
	return client, nil
}

func (p *GeminiProvider) Name() string { return p.name }

// Send performs a non-streaming chat completion.
func (p *GeminiProvider) Send(ctx context.Context, req Request) (*Response, error) {
	client, err := p.initClient(ctx)
	if err != nil {
		return nil, err
	}

	contents, config := p.buildRequest(req)
	resp, err := client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini send: %w", err)
	}
	return p.mapResponse(resp), nil
}

// Stream performs a streaming chat completion.
func (p *GeminiProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	client, err := p.initClient(ctx)
	if err != nil {
		return nil, err
	}

	contents, config := p.buildRequest(req)
	stream := client.Models.GenerateContentStream(ctx, req.Model, contents, config)
	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)

		var lastUsage *genai.GenerateContentResponseUsageMetadata

		for resp, err := range stream {
			select {
			case <-ctx.Done():
				ch <- StreamEvent{Error: ctx.Err()}
				return
			default:
			}

			if err != nil {
				ch <- StreamEvent{Error: fmt.Errorf("gemini stream: %w", err)}
				return
			}
			if resp == nil {
				continue
			}

			// Track latest usage metadata (only meaningful on final chunks).
			if resp.UsageMetadata != nil {
				lastUsage = resp.UsageMetadata
			}

			if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
				continue
			}

			for _, part := range resp.Candidates[0].Content.Parts {
				switch {
				case part.Thought && part.Text != "":
					ch <- StreamEvent{ReasoningDelta: part.Text}

				case part.Text != "":
					ch <- StreamEvent{TextDelta: part.Text}

				case part.FunctionCall != nil:
					fc := part.FunctionCall
					id := p.toolCallID(fc)
					ch <- StreamEvent{ToolCallStart: &ToolCall{Name: fc.Name, ID: id}}
					if len(fc.Args) > 0 {
						argsJSON, _ := json.Marshal(fc.Args)
						ch <- StreamEvent{ToolCallArgsDelta: string(argsJSON)}
					}
					ch <- StreamEvent{ToolCallEnd: &ToolCall{Name: fc.Name, ID: id, Args: fc.Args}}
				}
			}
		}

		// Emit final usage.
		if lastUsage != nil {
			ch <- StreamEvent{Done: &Usage{
				InputTokens:     int(lastUsage.PromptTokenCount),
				OutputTokens:    int(lastUsage.CandidatesTokenCount),
				CacheReadTokens: int(lastUsage.CachedContentTokenCount),
			}}
		}
	}()
	return ch, nil
}

// ListModels returns model IDs available for content generation.
func (p *GeminiProvider) ListModels(ctx context.Context) ([]string, error) {
	client, err := p.initClient(ctx)
	if err != nil {
		return nil, err
	}

	var ids []string
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			return ids, fmt.Errorf("gemini list models: %w", err)
		}
		if !supportsGenerateContent(model) {
			continue
		}
		id := model.Name
		// Remove "models/" prefix if present.
		if idx := strings.LastIndex(id, "/"); idx >= 0 && idx < len(id)-1 {
			id = id[idx+1:]
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// supportsGenerateContent returns true when the model supports the
// generateContent action (either explicitly listed or absent, which implies
// a content-generation model).
func supportsGenerateContent(m *genai.Model) bool {
	if m == nil || m.Name == "" {
		return false
	}
	// If the model explicitly lists actions, only include it if
	// generateContent is among them.
	if len(m.SupportedActions) > 0 {
		for _, action := range m.SupportedActions {
			if action == "generateContent" || action == "GenerateContent" {
				return true
			}
		}
		return false
	}
	// No explicit actions list means it's likely a generation model.
	return true
}

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

func (p *GeminiProvider) buildRequest(req Request) ([]*genai.Content, *genai.GenerateContentConfig) {
	config := &genai.GenerateContentConfig{}

	// System instruction.
	if req.SystemPrompt != "" {
		config.SystemInstruction = genai.NewContentFromText(req.SystemPrompt, genai.RoleUser)
	}

	// Tools.
	if len(req.Tools) > 0 {
		config.Tools = p.buildTools(req.Tools)
	}

	// Thinking.
	if thinking := p.buildThinking(req); thinking != nil {
		config.ThinkingConfig = thinking
	}

	// Build message history.
	// Keep a mapping from tool call IDs to tool names so tool-result messages
	// can carry the correct function name.
	toolCallNames := make(map[string]string) // callID -> name
	contents := make([]*genai.Content, 0, len(req.Messages))

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system", "compaction":
			// Accumulate into system instruction.
			if config.SystemInstruction == nil {
				config.SystemInstruction = genai.NewContentFromText("", genai.RoleUser)
			}
			config.SystemInstruction.Parts = append(config.SystemInstruction.Parts,
				genai.NewPartFromText(msg.Content))

		case "user":
			contents = append(contents, genai.NewContentFromText(msg.Content, genai.RoleUser))

		case "assistant":
			c := &genai.Content{Role: "model"}
			if msg.Content != "" {
				c.Parts = append(c.Parts, genai.NewPartFromText(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				toolCallNames[tc.ID] = tc.Name
				c.Parts = append(c.Parts,
					genai.NewPartFromFunctionCall(tc.Name, tc.Args))
			}
			// Preserve reasoning content as thought parts.
			if msg.ReasoningContent != "" {
				c.Parts = append(c.Parts, &genai.Part{
					Text:    msg.ReasoningContent,
					Thought: true,
				})
			}
			contents = append(contents, c)

		case "tool":
			name := toolCallNames[msg.ToolCallID]
			if name == "" {
				// If we have no prior mapping, fall back to a generic name.
				name = "unknown_function"
			}
			response := map[string]any{"output": msg.Content}
			fr := &genai.FunctionResponse{
				ID:       msg.ToolCallID,
				Name:     name,
				Response: response,
			}
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{FunctionResponse: fr}},
			})
		}
	}

	return contents, config
}

func (p *GeminiProvider) buildTools(tools []ToolDefinition) []*genai.Tool {
	decls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		decl := &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
		}
		if t.InputSchema != nil {
			// The Crobot tool schema is already a JSON Schema object. Use the
			// ParametersJsonSchema field for direct passthrough.
			// Remove the top-level "type" key if present (the SDK expects it
			// nested as an object schema), but typically the schema is
			// {"type":"object","properties":{...}} which works fine.
			decl.ParametersJsonSchema = t.InputSchema
		}
		decls = append(decls, decl)
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

func (p *GeminiProvider) buildThinking(req Request) *genai.ThinkingConfig {
	if req.Thinking == "" || req.Thinking == "none" {
		return nil
	}

	// Gemini 2.5 models use ThinkingBudget.
	// Gemini 3 models prefer ThinkingLevel.
	// We use a hybrid approach cap: set both and let the API pick.
	tc := &genai.ThinkingConfig{}

	switch req.Thinking {
	case "minimal":
		tc.ThinkingLevel = genai.ThinkingLevelMinimal
	case "low":
		tc.ThinkingLevel = genai.ThinkingLevelLow
	case "medium":
		tc.ThinkingLevel = genai.ThinkingLevelMedium
	case "high":
		tc.ThinkingLevel = genai.ThinkingLevelHigh
	case "xhigh":
		tc.ThinkingLevel = genai.ThinkingLevelHigh
	}

	// Set ThinkingBudget for models (like Gemini 2.5) that use it instead of levels.
	var budget int32
	switch req.Thinking {
	case "minimal":
		budget = 512
	case "low":
		budget = 1024
	case "medium":
		budget = 4096
	case "high":
		budget = 16384
	case "xhigh":
		budget = 32768
	}
	if budget > 0 {
		tc.ThinkingBudget = &budget
	}

	tc.IncludeThoughts = true
	return tc
}

// ---------------------------------------------------------------------------
// Response mapping
// ---------------------------------------------------------------------------

func (p *GeminiProvider) mapResponse(resp *genai.GenerateContentResponse) *Response {
	out := &Response{}

	if len(resp.Candidates) == 0 {
		return out
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil {
		return out
	}

	for _, part := range candidate.Content.Parts {
		switch {
		case part.Thought && part.Text != "":
			out.ReasoningContent += part.Text
		case part.Text != "":
			out.Text += part.Text
		case part.FunctionCall != nil:
			fc := part.FunctionCall
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:   p.toolCallID(fc),
				Name: fc.Name,
				Args: fc.Args,
			})
		}
	}

	if resp.UsageMetadata != nil {
		u := resp.UsageMetadata
		out.Usage = &Usage{
			InputTokens:     int(u.PromptTokenCount),
			OutputTokens:    int(u.CandidatesTokenCount),
			CacheReadTokens: int(u.CachedContentTokenCount),
		}
	}

	return out
}

// toolCallID returns the function call ID or synthesises a unique one.
// Gemini sometimes omits the ID field; we always set a stable one for the
// tool-result round-trip.
func (p *GeminiProvider) toolCallID(fc *genai.FunctionCall) string {
	if fc.ID != "" {
		return fc.ID
	}
	if fc.Name != "" {
		return "gemini_" + fc.Name
	}
	return "gemini_call_0"
}



// ---------------------------------------------------------------------------
// ModelInfoProvider
// ---------------------------------------------------------------------------

func (p *GeminiProvider) ListModelInfo(ctx context.Context) ([]ModelInfo, error) {
	client, err := p.initClient(ctx)
	if err != nil {
		return nil, err
	}

	var models []ModelInfo
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			return models, fmt.Errorf("gemini list models: %w", err)
		}
		if !supportsGenerateContent(model) {
			continue
		}
		id := model.Name
		if idx := strings.LastIndex(id, "/"); idx >= 0 && idx < len(id)-1 {
			id = id[idx+1:]
		}
		mi := ModelInfo{
			ID:            id,
			ContextLength: int(model.InputTokenLimit),
		}
		// Pricing is not available in the Model API response for Gemini API
		// (only Vertex AI returns it). Leave zero.
		models = append(models, mi)
	}
	return models, nil
}

// Ensure compile-time interface compliance.
var _ ModelInfoProvider = (*GeminiProvider)(nil)
