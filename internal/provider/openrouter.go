package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/models/operations"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
	"github.com/OpenRouterTeam/go-sdk/types/stream"
)

func init() {
	Register("openrouter", NewOpenRouter)
}

// OpenRouterProvider implements Provider using the official OpenRouter Go SDK.
type OpenRouterProvider struct {
	client *openrouter.OpenRouter
}

// NewOpenRouter creates a new OpenRouter provider with the given API key.
func NewOpenRouter(apiKey string) (Provider, error) {
	client := openrouter.New(openrouter.WithSecurity(apiKey))
	return &OpenRouterProvider{client: client}, nil
}

// Name returns the provider identifier.
func (p *OpenRouterProvider) Name() string { return "openrouter" }

// Send performs a non-streaming chat completion.
func (p *OpenRouterProvider) Send(ctx context.Context, req Request) (*Response, error) {
	chatReq := p.buildChatRequest(req)
	chatReq.Stream = openrouter.Pointer(false)

	res, err := p.client.Chat.Send(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter send: %w", err)
	}

	if res.ChatResult == nil {
		return nil, fmt.Errorf("openrouter: unexpected response type: %v", res.Type)
	}

	return p.mapResult(res.ChatResult), nil
}

// Stream performs a streaming chat completion. The returned channel emits
// StreamEvent values and is closed when streaming completes or fails.
func (p *OpenRouterProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	chatReq := p.buildChatRequest(req)
	chatReq.Stream = openrouter.Pointer(true)

	res, err := p.client.Chat.Send(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter stream: %w", err)
	}

	if res.EventStream == nil {
		return nil, fmt.Errorf("openrouter: expected event stream, got %v", res.Type)
	}

	ch := make(chan StreamEvent, 16)
	go p.streamLoop(ctx, res.EventStream, ch)
	return ch, nil
}

// ListModels returns all available model IDs from OpenRouter.
func (p *OpenRouterProvider) ListModels(ctx context.Context) ([]string, error) {
	res, err := p.client.Models.List(ctx, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("openrouter list models: %w", err)
	}
	var ids []string
	for _, m := range res.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// buildChatRequest converts our provider-agnostic Request into the OpenRouter SDK ChatRequest.
func (p *OpenRouterProvider) buildChatRequest(req Request) components.ChatRequest {
	var messages []components.ChatMessages

	// Always prepend the system prompt as the first message.
	if req.SystemPrompt != "" {
		messages = append(messages, components.CreateChatMessagesSystem(
			components.ChatSystemMessage{
				Content: components.CreateChatSystemMessageContentStr(req.SystemPrompt),
				Role:    components.ChatSystemMessageRoleSystem,
			},
		))
	}

	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			messages = append(messages, components.CreateChatMessagesUser(
				components.ChatUserMessage{
					Content: components.CreateChatUserMessageContentStr(m.Content),
					Role:    components.ChatUserMessageRoleUser,
				},
			))
		case "assistant":
			assistantContent := components.CreateChatAssistantMessageContentStr(m.Content)
			messages = append(messages, components.CreateChatMessagesAssistant(
				components.ChatAssistantMessage{
					Content:   optionalnullable.From(&assistantContent),
					Role:      components.ChatAssistantMessageRoleAssistant,
					ToolCalls: toChatToolCalls(m.ToolCalls),
				},
			))
		case "tool":
			messages = append(messages, components.CreateChatMessagesTool(
				components.ChatToolMessage{
					Content:    components.CreateChatToolMessageContentStr(m.Content),
					Role:       components.ChatToolMessageRoleTool,
					ToolCallID: m.ToolCallID,
				},
			))
		}
	}

	// Convert tool definitions.
	var tools []components.ChatFunctionTool
	for _, t := range req.Tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}
		}
		tools = append(tools, components.CreateChatFunctionToolChatFunctionToolFunction(
			components.ChatFunctionToolFunction{
				Function: components.ChatFunctionToolFunctionFunction{
					Name:        t.Name,
					Description: openrouter.Pointer(t.Description),
					Parameters:  params,
				},
				Type: components.ChatFunctionToolTypeFunction,
			},
		))
	}

	// Always include server tools for web search and datetime.
	tools = append(tools,
		components.CreateChatFunctionToolOpenRouterWebSearchServerTool(
			components.OpenRouterWebSearchServerTool{
				Type: components.OpenRouterWebSearchServerToolType("openrouter:web_search"),
			},
		),
		components.CreateChatFunctionToolDatetimeServerTool(
			components.DatetimeServerTool{
				Type: components.DatetimeServerToolType("openrouter:datetime"),
				Parameters: &components.DatetimeServerToolConfig{
					Timezone: openrouter.Pointer("UTC"),
				},
			},
		),
	)

	chatReq := components.ChatRequest{
		Model:    openrouter.Pointer(req.Model),
		Messages: messages,
		Tools:    tools,
	}
	if req.Thinking != "" && req.Thinking != "none" {
		effort := components.Effort(req.Thinking)
		chatReq.Reasoning = &components.Reasoning{
			Effort: optionalnullable.From(&effort),
		}
	}
	return chatReq
}

// mapResult converts an OpenRouter ChatResult to our Response type.
func (p *OpenRouterProvider) mapResult(result *components.ChatResult) *Response {
	resp := &Response{}
	if len(result.Choices) > 0 {
		choice := result.Choices[0]
		if contentVal, ok := choice.Message.Content.Get(); ok {
			// The content is a ChatAssistantMessageContent union.
			// Extract the string part.
			if contentVal.Str != nil {
				resp.Text = *contentVal.Str
			}
		}
		if reasonVal, ok := choice.Message.Reasoning.Get(); ok && reasonVal != nil {
			resp.Text += *reasonVal
		}
		for _, tc := range choice.Message.ToolCalls {
			args := parseToolArgs(tc.Function.Arguments)
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				Name: tc.Function.Name,
				ID:   tc.ID,
				Args: args,
			})
		}
	}
	if result.Usage != nil {
		resp.Usage = &Usage{
			InputTokens:  int(result.Usage.PromptTokens),
			OutputTokens: int(result.Usage.CompletionTokens),
		}
	}
	return resp
}

// streamLoop reads SSE events from the EventStream and converts them to StreamEvent on the channel.
func (p *OpenRouterProvider) streamLoop(
	ctx context.Context,
	es *stream.EventStream[operations.SendChatCompletionRequestResponseBody],
	ch chan<- StreamEvent,
) {
	defer close(ch)

	// Track tool calls being built up across chunks.
	type pendingTool struct {
		name string
		id   string
		args strings.Builder
	}
	toolAccum := map[int64]*pendingTool{} // index -> pending

	for es.Next() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}

		val := es.Value()
		if val == nil {
			continue
		}

		chunk := &val.Data

		if chunk.Error != nil {
			ch <- StreamEvent{Error: fmt.Errorf("openrouter: %s", chunk.Error.Message)}
			continue
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Reasoning delta. Prefer the flat field when present; otherwise fall
			// back to reasoning_details, which some OpenRouter-routed models stream.
			if reasoning := extractOpenRouterReasoningDelta(delta); reasoning != "" {
				ch <- StreamEvent{ReasoningDelta: reasoning}
			}

			// Text delta.
			if contentPtr, ok := delta.Content.Get(); ok && contentPtr != nil && *contentPtr != "" {
				ch <- StreamEvent{TextDelta: *contentPtr}
			}

			// Tool call deltas.
			for _, tc := range delta.ToolCalls {
				idx := tc.Index
				id := ""
				if tc.ID != nil {
					id = *tc.ID
				}
				name := ""
				if tc.Function != nil && tc.Function.Name != nil {
					name = *tc.Function.Name
				}

				p, exists := toolAccum[idx]
				started := exists && p.name != "" && p.id != ""
				if !exists {
					toolAccum[idx] = &pendingTool{}
					p = toolAccum[idx]
				}
				if id != "" {
					p.id = id
				}
				if name != "" {
					p.name = name
				}
				if !started && p.name != "" && p.id != "" {
					ch <- StreamEvent{ToolCallStart: &ToolCall{Name: p.name, ID: p.id}}
				}

				// Append args delta.
				if tc.Function != nil && tc.Function.Arguments != nil {
					p.args.WriteString(*tc.Function.Arguments)
					ch <- StreamEvent{ToolCallArgsDelta: *tc.Function.Arguments}
				}
			}

			// Finish reason: flush accumulated tool calls.
			if choice.FinishReason != nil {
				reason := *choice.FinishReason
				switch reason {
				case components.ChatFinishReasonEnumToolCalls,
					components.ChatFinishReasonEnumStop,
					components.ChatFinishReasonEnumLength,
					components.ChatFinishReasonEnumContentFilter:
					for idx, p := range toolAccum {
						_ = idx
						if p.name != "" {
							args := parseToolArgs(p.args.String())
							ch <- StreamEvent{ToolCallEnd: &ToolCall{
								Name: p.name,
								ID:   p.id,
								Args: args,
							}}
						}
					}
					toolAccum = map[int64]*pendingTool{}
				}
			}
		}

		// Usage (at stream end).
		if chunk.Usage != nil {
			ch <- StreamEvent{Done: &Usage{
				InputTokens:  int(chunk.Usage.PromptTokens),
				OutputTokens: int(chunk.Usage.CompletionTokens),
			}}
		}
	}

	if err := es.Err(); err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("openrouter stream: %w", err)}
	}
}

func extractOpenRouterReasoningDelta(delta components.ChatStreamDelta) string {
	if reasonPtr, ok := delta.Reasoning.Get(); ok && reasonPtr != nil && *reasonPtr != "" {
		return *reasonPtr
	}

	var out strings.Builder
	for _, detail := range delta.ReasoningDetails {
		if detail.ReasoningDetailText != nil {
			if textPtr, ok := detail.ReasoningDetailText.Text.Get(); ok && textPtr != nil && *textPtr != "" {
				out.WriteString(*textPtr)
			}
		}
		if detail.ReasoningDetailSummary != nil && detail.ReasoningDetailSummary.Summary != "" {
			out.WriteString(detail.ReasoningDetailSummary.Summary)
		}
	}
	return out.String()
}

func toChatToolCalls(calls []ToolCall) []components.ChatToolCall {
	out := make([]components.ChatToolCall, 0, len(calls))
	for _, tc := range calls {
		args, _ := json.Marshal(tc.Args)
		out = append(out, components.ChatToolCall{
			ID:   tc.ID,
			Type: components.ChatToolCallTypeFunction,
			Function: components.ChatToolCallFunction{
				Name:      tc.Name,
				Arguments: string(args),
			},
		})
	}
	return out
}

// parseToolArgs attempts to parse a JSON string into map[string]any.
func parseToolArgs(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]any{"raw": raw}
	}
	return args
}
