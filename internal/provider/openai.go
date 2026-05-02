package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
)

func init() {
	Register("openai", NewOpenAI)
	Register("openai-codex", NewOpenAIOAuth)
	Register("deepseek", NewDeepSeek)
}

const openAIBaseURL = "https://api.openai.com/v1"
const deepSeekBaseURL = "https://api.deepseek.com"

// OpenAIProvider implements Provider using the official openai-go SDK.
type OpenAIProvider struct {
	name   string
	apiKey string
	client openai.Client
}

func NewOpenAI(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai: missing API key or OAuth access token")
	}
	return &OpenAIProvider{
		name:   "openai",
		apiKey: apiKey,
		client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(openAIBaseURL),
		),
	}, nil
}

func NewOpenAIOAuth(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai-codex: missing OAuth access token")
	}
	return &OpenAIProvider{
		name:   "openai-codex",
		apiKey: apiKey,
		client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(openAIBaseURL),
		),
	}, nil
}

func NewDeepSeek(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("deepseek: missing API key")
	}
	return &OpenAIProvider{
		name:   "deepseek",
		apiKey: apiKey,
		client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(deepSeekBaseURL),
		),
	}, nil
}

func (p *OpenAIProvider) Name() string { return p.name }

// Send performs a non-streaming chat completion.
func (p *OpenAIProvider) Send(ctx context.Context, req Request) (*Response, error) {
	params := p.toChatParams(req, false)
	result, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("%s send: %w", p.name, err)
	}
	return p.mapResponse(result), nil
}

// Stream performs a streaming chat completion.
func (p *OpenAIProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	params := p.toChatParams(req, true)
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	ch := make(chan StreamEvent, 16)
	go p.streamLoop(ctx, stream, ch)
	return ch, nil
}

// ListModels returns available model IDs.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	if p.name == "deepseek" {
		return []string{"deepseek-v4-pro", "deepseek-v4-flash"}, nil
	}
	if p.name == "openai-codex" || isOpenAIOAuthToken(p.apiKey) {
		return openAIOAuthModels(), nil
	}

	page, err := p.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s list models: %w", p.name, err)
	}
	ids := make([]string, 0, len(page.Data))
	for _, m := range page.Data {
		ids = append(ids, m.ID)
	}
	if len(ids) == 0 {
		// Fallback: list page is empty
		return openAIOAuthModels(), nil
	}
	return ids, nil
}

// toChatParams converts a provider-agnostic Request into the SDK's ChatCompletionNewParams.
func (p *OpenAIProvider) toChatParams(req Request, stream bool) openai.ChatCompletionNewParams {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}
	for _, m := range req.Messages {
		messages = append(messages, p.convertMessage(m, req))
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}

	if len(req.Tools) > 0 {
		params.Tools = p.convertTools(req.Tools)
	}

	// Set reasoning effort for thinking.
	p.setReasoningEffort(&params, req)

	return params
}

func (p *OpenAIProvider) convertMessage(m Message, req Request) openai.ChatCompletionMessageParamUnion {
	switch m.Role {
	case "assistant":
		if len(m.ToolCalls) > 0 {
			assistant := &openai.ChatCompletionAssistantMessageParam{}
			if m.Content != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(m.Content),
				}
			}
			if p.shouldSendReasoningContent(m, req) {
				assistant.SetExtraFields(map[string]any{
					"reasoning_content": m.ReasoningContent,
				})
			}
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Args)
				assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: string(args),
						},
					},
				})
			}
			return openai.ChatCompletionMessageParamUnion{OfAssistant: assistant}
		}
		assistant := &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(m.Content),
			},
		}
		if p.shouldSendReasoningContent(m, req) {
			assistant.SetExtraFields(map[string]any{
				"reasoning_content": m.ReasoningContent,
			})
		}
		return openai.ChatCompletionMessageParamUnion{OfAssistant: assistant}

	case "tool":
		return openai.ToolMessage(m.Content, m.ToolCallID)

	case "user":
		return openai.UserMessage(m.Content)

	default:
		return openai.UserMessage(m.Content)
	}
}

func (p *OpenAIProvider) shouldSendReasoningContent(m Message, req Request) bool {
	if m.ReasoningContent != "" {
		return true
	}
	// DeepSeek's thinking-mode tool-call protocol requires the assistant
	// reasoning_content field to be present on every historical assistant
	// message that made tool calls. Some responses can carry an empty
	// reasoning_content; preserving the field (even as "") prevents DeepSeek
	// from rejecting the next tool-call turn as missing reasoning_content.
	return p.name == "deepseek" && deepSeekThinkingEnabled(req.Thinking) && len(m.ToolCalls) > 0
}

func (p *OpenAIProvider) convertTools(tools []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		}
		result = append(result, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        t.Name,
					Description: openai.String(t.Description),
					Parameters:  shared.FunctionParameters(params),
				},
			},
		})
	}
	return result
}

func (p *OpenAIProvider) setReasoningEffort(params *openai.ChatCompletionNewParams, req Request) {
	if p.name == "openrouter" {
		if req.Thinking == "" {
			return
		}
		params.SetExtraFields(map[string]any{
			"reasoning": map[string]any{"effort": req.Thinking},
		})
		return
	}
	if p.name == "deepseek" {
		if !deepSeekThinkingEnabled(req.Thinking) {
			params.SetExtraFields(map[string]any{
				"thinking": map[string]any{"type": "disabled"},
			})
			return
		}
		params.SetExtraFields(map[string]any{
			"thinking": map[string]any{"type": "enabled"},
		})
		if effort, ok := deepSeekReasoningEffort(req.Thinking); ok {
			params.ReasoningEffort = shared.ReasoningEffort(effort)
		}
		return
	}
	if effort, ok := openAIReasoningEffort(req.Model, req.Thinking); ok {
		params.ReasoningEffort = shared.ReasoningEffort(effort)
	}
}

func deepSeekThinkingEnabled(thinking string) bool {
	switch thinking {
	case "", "none", "disabled":
		return false
	default:
		return true
	}
}

func deepSeekReasoningEffort(thinking string) (string, bool) {
	switch thinking {
	case "", "none", "disabled":
		return "", false
	case "minimal", "low", "medium":
		return "high", true
	case "high":
		return "high", true
	case "xhigh", "max":
		return "max", true
	default:
		return thinking, true
	}
}

func openAIReasoningEffort(modelID, thinking string) (string, bool) {
	if thinking == "" {
		return "", false
	}
	id := modelID
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		id = id[idx+1:]
	}

	if (strings.HasPrefix(id, "gpt-5.2") || strings.HasPrefix(id, "gpt-5.3") || strings.HasPrefix(id, "gpt-5.4") || strings.HasPrefix(id, "gpt-5.5")) && thinking == "minimal" {
		return "low", true
	}
	if id == "gpt-5.1" && thinking == "xhigh" {
		return "high", true
	}
	if id == "gpt-5.1-codex-mini" {
		if thinking == "high" || thinking == "xhigh" {
			return "high", true
		}
		return "medium", true
	}
	return thinking, true
}

func isOpenAIOAuthToken(token string) bool {
	return !strings.HasPrefix(token, "sk-")
}

func openAIOAuthModels() []string {
	return []string{
		"gpt-5.1",
		"gpt-5.2",
		"gpt-5.3",
		"gpt-5.4",
		"gpt-5.5",
	}
}

// mapResponse converts an SDK ChatCompletion into our Response type.
func (p *OpenAIProvider) mapResponse(c *openai.ChatCompletion) *Response {
	out := &Response{}
	if len(c.Choices) > 0 {
		msg := c.Choices[0].Message
		out.Text = msg.Content
		out.ReasoningContent = extractOpenAIMessageReasoning(msg)
		for _, tc := range msg.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				Name: tc.Function.Name,
				ID:   tc.ID,
				Args: parseToolArgs(tc.Function.Arguments),
			})
		}
	}
	if c.Usage.PromptTokens > 0 || c.Usage.CompletionTokens > 0 {
		out.Usage = &Usage{
			InputTokens:     int(c.Usage.PromptTokens),
			OutputTokens:    int(c.Usage.CompletionTokens),
			CacheReadTokens: int(c.Usage.PromptTokensDetails.CachedTokens),
		}
	}
	return out
}

// streamLoop reads SSE chunks from the SDK stream and emits StreamEvents.
func (p *OpenAIProvider) streamLoop(ctx context.Context, stream interface {
	Next() bool
	Current() openai.ChatCompletionChunk
	Err() error
}, ch chan<- StreamEvent) {
	defer close(ch)

	type pendingTool struct {
		name string
		id   string
		args strings.Builder
	}
	pending := map[int64]*pendingTool{}

	for stream.Next() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}

		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- StreamEvent{TextDelta: choice.Delta.Content}
			}

			if reasoning := extractOpenAIReasoningDelta(choice.Delta); reasoning != "" {
				ch <- StreamEvent{ReasoningDelta: reasoning}
			}

			for _, tc := range choice.Delta.ToolCalls {
				pnd := pending[tc.Index]
				if pnd == nil {
					pnd = &pendingTool{}
					pending[tc.Index] = pnd
				}
				wasStarted := pnd.name != "" && pnd.id != ""
				if tc.ID != "" {
					pnd.id = tc.ID
				}
				if tc.Function.Name != "" {
					pnd.name = tc.Function.Name
				}
				if !wasStarted && pnd.name != "" && pnd.id != "" {
					ch <- StreamEvent{ToolCallStart: &ToolCall{Name: pnd.name, ID: pnd.id}}
				}
				if tc.Function.Arguments != "" {
					pnd.args.WriteString(tc.Function.Arguments)
					ch <- StreamEvent{ToolCallArgsDelta: tc.Function.Arguments}
				}
			}

			if choice.FinishReason != "" {
				for _, pnd := range pending {
					if pnd.name != "" {
						ch <- StreamEvent{ToolCallEnd: &ToolCall{
							Name: pnd.name,
							ID:   pnd.id,
							Args: parseToolArgs(pnd.args.String()),
						}}
					}
				}
				pending = map[int64]*pendingTool{}
			}
		}

		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			ch <- StreamEvent{Done: &Usage{
				InputTokens:     int(chunk.Usage.PromptTokens),
				OutputTokens:    int(chunk.Usage.CompletionTokens),
				CacheReadTokens: int(chunk.Usage.PromptTokensDetails.CachedTokens),
			}}
		}
	}

	if err := stream.Err(); err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("%s stream: %w", p.name, err)}
	}
}

func extractOpenAIReasoningDelta(delta openai.ChatCompletionChunkChoiceDelta) string {
	// openai-go v3.33.0 does not expose OpenAI-compatible reasoning fields as
	// typed struct members, but unknown JSON fields are preserved in ExtraFields.
	return extractOpenAIReasoningFields(delta.JSON.ExtraFields)
}

func extractOpenAIMessageReasoning(msg openai.ChatCompletionMessage) string {
	return extractOpenAIReasoningFields(msg.JSON.ExtraFields)
}

func parseToolArgs(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{"raw": raw}
	}
	return m
}

func extractOpenAIReasoningFields(fields map[string]respjson.Field) string {
	// DeepSeek, OpenRouter, and several OpenAI-compatible endpoints stream
	// reasoning under one of these flat aliases.
	for _, name := range []string{"reasoning_content", "reasoning", "reasoning_text"} {
		field, ok := fields[name]
		if !ok || field.Raw() == "" || field.Raw() == "null" {
			continue
		}
		var reasoning string
		if err := json.Unmarshal([]byte(field.Raw()), &reasoning); err == nil && reasoning != "" {
			return reasoning
		}
	}

	// OpenRouter may also normalize reasoning into reasoning_details.
	field, ok := fields["reasoning_details"]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return ""
	}
	var details []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(field.Raw()), &details); err != nil {
		return ""
	}
	var out strings.Builder
	for _, detail := range details {
		switch detail.Type {
		case "reasoning.text":
			out.WriteString(detail.Text)
		case "reasoning.summary":
			out.WriteString(detail.Summary)
		}
	}
	return out.String()
}
