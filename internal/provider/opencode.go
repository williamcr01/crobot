package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOpt "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
)

func init() {
	Register("opencode-zen", NewOpenCodeZen)
	Register("opencode-go", NewOpenCodeGo)
}

const opencodeBaseURL = "https://opencode.ai/zen/v1"

// OpenCodeProvider implements Provider for OpenCode Zen and OpenCode Go.
// Both services share the same API endpoints but use different API keys
// (pay-as-you-go vs subscription).
//
// Endpoint routing based on model ID:
//   - claude-* -> Anthropic Messages API (/v1/messages)
//   - gpt-*    -> OpenAI Responses API  (/v1/responses)
//   - *        -> OpenAI Chat Completions (/v1/chat/completions)
type OpenCodeProvider struct {
	name   string
	apiKey string

	lazy       sync.Once
	chatClient *openai.Client // for /v1/chat/completions
	anthClient anthropic.Client
	httpClient *http.Client // for responses API & model listing
}

func NewOpenCodeZen(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("opencode-zen: missing API key")
	}
	return &OpenCodeProvider{name: "opencode-zen", apiKey: apiKey, httpClient: &http.Client{Timeout: 0}}, nil
}

func NewOpenCodeGo(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("opencode-go: missing API key")
	}
	return &OpenCodeProvider{name: "opencode-go", apiKey: apiKey, httpClient: &http.Client{Timeout: 0}}, nil
}

func (p *OpenCodeProvider) Name() string { return p.name }

func (p *OpenCodeProvider) initClients() {
	p.lazy.Do(func() {
		cli := openai.NewClient(
			option.WithAPIKey(p.apiKey),
			option.WithBaseURL(opencodeBaseURL),
		)
		p.chatClient = &cli
		p.anthClient = anthropic.NewClient(
			anthropicOpt.WithAPIKey(p.apiKey),
			anthropicOpt.WithBaseURL(opencodeBaseURL+"/"),
		)
		if p.httpClient == nil {
			p.httpClient = &http.Client{Timeout: 0}
		}
	})
}

// -------- Send --------

func (p *OpenCodeProvider) Send(ctx context.Context, req Request) (*Response, error) {
	p.initClients()

	switch p.modelEndpoint(req.Model) {
	case endpointAnthropic:
		msg, err := p.anthClient.Messages.New(ctx, p.toAnthropicParams(req))
		if err != nil {
			return nil, fmt.Errorf("%s send: %w", p.name, err)
		}
		return p.mapAnthropicResponse(msg), nil

	case endpointResponses:
		stream, err := p.streamResponsesRaw(ctx, req)
		if err != nil {
			return nil, err
		}
		resp := &Response{}
		for ev := range stream {
			if ev.Error != nil {
				return nil, ev.Error
			}
			resp.Text += ev.TextDelta
			resp.ReasoningContent += ev.ReasoningDelta
			if ev.ToolCallEnd != nil {
				resp.ToolCalls = append(resp.ToolCalls, *ev.ToolCallEnd)
			}
			if ev.Done != nil {
				resp.Usage = ev.Done
			}
		}
		return resp, nil

	default: // chat completions
		params := p.toChatParams(req, false)
		result, err := p.chatClient.Chat.Completions.New(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("%s send: %w", p.name, err)
		}
		return p.mapChatResponse(result), nil
	}
}

// -------- Stream --------

func (p *OpenCodeProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	p.initClients()

	switch p.modelEndpoint(req.Model) {
	case endpointAnthropic:
		stream := p.anthClient.Messages.NewStreaming(ctx, p.toAnthropicParams(req))
		ch := make(chan StreamEvent, 16)
		go p.streamAnthropic(ctx, stream, ch)
		return ch, nil

	case endpointResponses:
		return p.streamResponsesRaw(ctx, req)

	default: // chat completions
		params := p.toChatParams(req, true)
		stream := p.chatClient.Chat.Completions.NewStreaming(ctx, params)
		ch := make(chan StreamEvent, 16)
		go p.streamChat(ctx, stream, ch)
		return ch, nil
	}
}

// -------- Model listing --------

func (p *OpenCodeProvider) ListModels(ctx context.Context) ([]string, error) {
	models, err := p.ListModelInfo(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids, nil
}

func (p *OpenCodeProvider) ListModelInfo(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opencodeBaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s list models: %w", p.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return p.defaultModels(), nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s list models read: %w", p.name, err)
	}

	// Try: { data: [{ id, context_length, input_price, output_price }] }
	var data struct {
		Data []struct {
			ID            string  `json:"id"`
			ContextLength int     `json:"context_length"`
			InputPrice    float64 `json:"input_price"`
			OutputPrice   float64 `json:"output_price"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &data); err == nil && len(data.Data) > 0 {
		return p.buildModelInfo(data.Data), nil
	}

	// Try flat array: [{ id, context_length, ... }]
	var flat []struct {
		ID            string  `json:"id"`
		ContextLength int     `json:"context_length"`
		InputPrice    float64 `json:"input_price"`
		OutputPrice   float64 `json:"output_price"`
	}
	if err := json.Unmarshal(raw, &flat); err == nil && len(flat) > 0 {
		return p.buildModelInfo(flat), nil
	}

	return p.defaultModels(), nil
}

func (p *OpenCodeProvider) buildModelInfo(items []struct {
	ID            string  `json:"id"`
	ContextLength int     `json:"context_length"`
	InputPrice    float64 `json:"input_price"`
	OutputPrice   float64 `json:"output_price"`
}) []ModelInfo {
	models := make([]ModelInfo, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		models = append(models, ModelInfo{
			ID:            item.ID,
			ContextLength: item.ContextLength,
			Pricing: Pricing{
				InputPerMTok:  item.InputPrice,
				OutputPerMTok: item.OutputPrice,
			},
		})
	}
	if len(models) > 0 {
		return models
	}
	return p.defaultModels()
}

func (p *OpenCodeProvider) defaultModels() []ModelInfo {
	known := []struct {
		id     string
		ctx    int
		input  float64
		output float64
	}{
		// GPT-5.x models (Responses API)
		{"gpt-5.5", 272000, 5.00, 30.00},
		{"gpt-5.5-pro", 272000, 30.00, 180.00},
		{"gpt-5.4", 272000, 2.50, 15.00},
		{"gpt-5.4-pro", 272000, 30.00, 180.00},
		{"gpt-5.4-mini", 272000, 0.75, 4.50},
		{"gpt-5.4-nano", 272000, 0.20, 1.25},
		{"gpt-5.3-codex", 200000, 1.75, 14.00},
		{"gpt-5.3-codex-spark", 200000, 1.75, 14.00},
		{"gpt-5.2", 200000, 1.75, 14.00},
		{"gpt-5.2-codex", 200000, 1.75, 14.00},
		{"gpt-5.1", 200000, 1.07, 8.50},
		{"gpt-5.1-codex", 200000, 1.07, 8.50},
		{"gpt-5.1-codex-max", 200000, 1.25, 10.00},
		{"gpt-5.1-codex-mini", 200000, 0.25, 2.00},
		{"gpt-5", 200000, 1.07, 8.50},
		{"gpt-5-codex", 200000, 1.07, 8.50},
		{"gpt-5-nano", 200000, 0, 0},
		// Claude models (Messages API)
		{"claude-opus-4-7", 200000, 5.00, 25.00},
		{"claude-opus-4-6", 200000, 5.00, 25.00},
		{"claude-opus-4-5", 200000, 5.00, 25.00},
		{"claude-opus-4-1", 200000, 15.00, 75.00},
		{"claude-sonnet-4-6", 200000, 3.00, 15.00},
		{"claude-sonnet-4-5", 200000, 3.00, 15.00},
		{"claude-sonnet-4", 200000, 3.00, 15.00},
		{"claude-haiku-4-5", 200000, 1.00, 5.00},
		{"claude-3-5-haiku", 200000, 1.00, 5.00},
		// Qwen models (Chat Completions)
		{"qwen3.6-plus", 200000, 0.50, 3.00},
		{"qwen3.5-plus", 200000, 0.20, 1.20},
		// MiniMax models (Chat Completions)
		{"minimax-m2.7", 200000, 0.30, 1.20},
		{"minimax-m2.5", 200000, 0.30, 1.20},
		{"minimax-m2.5-free", 200000, 0, 0},
		// GLM models (Chat Completions)
		{"glm-5.1", 200000, 1.40, 4.40},
		{"glm-5", 200000, 1.00, 3.20},
		// Kimi models (Chat Completions)
		{"kimi-k2.5", 200000, 0.60, 3.00},
		{"kimi-k2.6", 200000, 0.95, 4.00},
		// Free models
		{"big-pickle", 200000, 0, 0},
		{"ling-2.6-flash", 200000, 0, 0},
		{"hy3-preview-free", 200000, 0, 0},
		{"nemotron-3-super-free", 200000, 0, 0},
	}
	models := make([]ModelInfo, 0, len(known))
	for _, m := range known {
		models = append(models, ModelInfo{
			ID:            m.id,
			ContextLength: m.ctx,
			Pricing: Pricing{
				InputPerMTok:  m.input,
				OutputPerMTok: m.output,
			},
		})
	}
	return models
}

// -------- endpoint routing --------

type endpoint int

const (
	endpointChat       endpoint = iota
	endpointResponses
	endpointAnthropic
)

func (p *OpenCodeProvider) modelEndpoint(modelID string) endpoint {
	id := modelID
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		id = id[idx+1:]
	}
	switch {
	case strings.HasPrefix(id, "claude-"):
		return endpointAnthropic
	case strings.HasPrefix(id, "gpt-"):
		return endpointResponses
	default:
		return endpointChat
	}
}

// -------- Chat Completions (OpenAI-compatible) --------

func (p *OpenCodeProvider) toChatParams(req Request, stream bool) openai.ChatCompletionNewParams {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}
	for _, m := range req.Messages {
		messages = append(messages, opencodeChatMessage(m))
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Model),
		Messages: messages,
	}
	if len(req.Tools) > 0 {
		params.Tools = opencodeChatTools(req.Tools)
	}
	return params
}

func opencodeChatMessage(m Message) openai.ChatCompletionMessageParamUnion {
	switch m.Role {
	case "assistant":
		assistant := &openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(m.Content),
			},
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
	case "tool":
		return openai.ToolMessage(m.Content, m.ToolCallID)
	case "user":
		return openai.UserMessage(m.Content)
	default:
		return openai.UserMessage(m.Content)
	}
}

func opencodeChatTools(tools []ToolDefinition) []openai.ChatCompletionToolUnionParam {
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

func (p *OpenCodeProvider) mapChatResponse(c *openai.ChatCompletion) *Response {
	out := &Response{}
	if len(c.Choices) > 0 {
		msg := c.Choices[0].Message
		out.Text = msg.Content
		out.ReasoningContent = extractOpenCodeReasoning(msg.JSON.ExtraFields)
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

// -------- Chat Completions streaming loop --------

type opencodeChatStream interface {
	Next() bool
	Current() openai.ChatCompletionChunk
	Err() error
}

func (p *OpenCodeProvider) streamChat(ctx context.Context, stream opencodeChatStream, ch chan<- StreamEvent) {
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
			if reasoning := extractOpenCodeReasoning(choice.Delta.JSON.ExtraFields); reasoning != "" {
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

func extractOpenCodeReasoning(fields map[string]respjson.Field) string {
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
	return ""
}

// -------- Anthropic Messages API --------

func (p *OpenCodeProvider) toAnthropicParams(req Request) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		MaxTokens: 8192,
		Messages:  make([]anthropic.MessageParam, 0, len(req.Messages)),
		Model:     anthropic.Model(req.Model),
	}

	if req.SystemPrompt != "" {
		params.System = append(params.System, anthropic.TextBlockParam{Text: req.SystemPrompt})
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system", "compaction":
			if msg.Content != "" {
				params.System = append(params.System, anthropic.TextBlockParam{Text: msg.Content})
			}
		case "user":
			params.Messages = append(params.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case "assistant":
			blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(msg.ToolCalls))
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Args, tc.Name))
			}
			if len(blocks) > 0 {
				params.Messages = append(params.Messages, anthropic.NewAssistantMessage(blocks...))
			}
		case "tool":
			params.Messages = append(params.Messages, anthropic.NewUserMessage(anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false)))
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = opencodeAnthropicTools(req.Tools)
	}

	if req.Metadata != nil {
		if userID, ok := req.Metadata["user_id"]; ok && userID != "" {
			params.Metadata = anthropic.MetadataParam{
				UserID: anthropic.String(userID),
			}
		}
	}

	return params
}

func opencodeAnthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		var props any
		var required []string
		extra := map[string]any{}
		if t.InputSchema != nil {
			if p, ok := t.InputSchema["properties"]; ok {
				props = p
			}
			if r, ok := t.InputSchema["required"]; ok {
				if rs, ok := r.([]any); ok {
					for _, v := range rs {
						if s, ok := v.(string); ok {
							required = append(required, s)
						}
					}
				}
			}
			for k, v := range t.InputSchema {
				if k != "properties" && k != "required" && k != "type" {
					extra[k] = v
				}
			}
		}
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties:  props,
				Required:    required,
				ExtraFields: extra,
			},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out
}

func (p *OpenCodeProvider) mapAnthropicResponse(msg *anthropic.Message) *Response {
	out := &Response{}
	if msg == nil {
		return out
	}
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Text += b.Text
		case anthropic.ToolUseBlock:
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				Name: b.Name,
				ID:   b.ID,
				Args: parseToolArgs(string(b.Input)),
			})
		}
	}
	if msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0 {
		out.Usage = &Usage{
			InputTokens:      int(msg.Usage.InputTokens),
			OutputTokens:     int(msg.Usage.OutputTokens),
			CacheReadTokens:  int(msg.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(msg.Usage.CacheCreationInputTokens),
		}
	}
	return out
}

type opencodeAnthropicStream interface {
	Next() bool
	Current() anthropic.MessageStreamEventUnion
	Err() error
}

func (p *OpenCodeProvider) streamAnthropic(ctx context.Context, stream opencodeAnthropicStream, ch chan<- StreamEvent) {
	defer close(ch)

	type pendingTool struct {
		id   string
		name string
		args strings.Builder
	}
	pending := map[int64]*pendingTool{}
	var usage *Usage

	for stream.Next() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}

		event := stream.Current()
		switch ev := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			usage = &Usage{
				InputTokens:      int(ev.Message.Usage.InputTokens),
				OutputTokens:     int(ev.Message.Usage.OutputTokens),
				CacheReadTokens:  int(ev.Message.Usage.CacheReadInputTokens),
				CacheWriteTokens: int(ev.Message.Usage.CacheCreationInputTokens),
			}
		case anthropic.MessageDeltaEvent:
			if usage == nil {
				usage = &Usage{}
			}
			usage.InputTokens = int(ev.Usage.InputTokens)
			usage.OutputTokens = int(ev.Usage.OutputTokens)
			usage.CacheReadTokens = int(ev.Usage.CacheReadInputTokens)
			usage.CacheWriteTokens = int(ev.Usage.CacheCreationInputTokens)
		case anthropic.ContentBlockStartEvent:
			switch block := ev.ContentBlock.AsAny().(type) {
			case anthropic.ToolUseBlock:
				pnd := &pendingTool{id: block.ID, name: block.Name}
				if len(block.Input) > 0 && string(block.Input) != "{}" {
					pnd.args.Write(block.Input)
				}
				pending[ev.Index] = pnd
				ch <- StreamEvent{ToolCallStart: &ToolCall{Name: pnd.name, ID: pnd.id}}
				if pnd.args.Len() > 0 {
					ch <- StreamEvent{ToolCallArgsDelta: pnd.args.String()}
				}
			}
		case anthropic.ContentBlockDeltaEvent:
			switch delta := ev.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if delta.Text != "" {
					ch <- StreamEvent{TextDelta: delta.Text}
				}
			case anthropic.ThinkingDelta:
				if delta.Thinking != "" {
					ch <- StreamEvent{ReasoningDelta: delta.Thinking}
				}
			case anthropic.InputJSONDelta:
				if delta.PartialJSON != "" {
					pnd := pending[ev.Index]
					if pnd == nil {
						pnd = &pendingTool{}
						pending[ev.Index] = pnd
					}
					pnd.args.WriteString(delta.PartialJSON)
					ch <- StreamEvent{ToolCallArgsDelta: delta.PartialJSON}
				}
			}
		case anthropic.ContentBlockStopEvent:
			if pnd := pending[ev.Index]; pnd != nil && pnd.name != "" {
				ch <- StreamEvent{ToolCallEnd: &ToolCall{Name: pnd.name, ID: pnd.id, Args: parseToolArgs(pnd.args.String())}}
				delete(pending, ev.Index)
			}
		}
	}

	if err := stream.Err(); err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("%s stream: %w", p.name, err)}
		return
	}
	if usage != nil {
		ch <- StreamEvent{Done: usage}
	}
}

// -------- Responses API (raw HTTP) --------

func (p *OpenCodeProvider) streamResponsesRaw(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	body := p.buildResponsesBody(req)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s responses marshal: %w", p.name, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, opencodeBaseURL+"/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if userAgent := metadataUserAgent(req.Metadata); userAgent != "" {
		httpReq.Header.Set("User-Agent", userAgent)
	}

	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			ch <- StreamEvent{Error: fmt.Errorf("%s responses stream: %w", p.name, err)}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			r, _ := io.ReadAll(resp.Body)
			ch <- StreamEvent{Error: fmt.Errorf("%s responses stream: POST %q: %s %s", p.name, opencodeBaseURL+"/responses", resp.Status, strings.TrimSpace(string(r)))}
			return
		}
		p.readResponsesSSE(ctx, resp.Body, ch)
	}()
	return ch, nil
}

func (p *OpenCodeProvider) buildResponsesBody(req Request) map[string]any {
	body := map[string]any{
		"model":        req.Model,
		"store":        false,
		"stream":       true,
		"instructions": req.SystemPrompt,
		"input":        p.responsesInput(req.Messages),
		"tool_choice":  "auto",
		"text":         map[string]any{"verbosity": "low"},
	}

	if len(req.Tools) > 0 {
		body["tools"] = p.responsesTools(req.Tools)
	}

	return body
}

func (p *OpenCodeProvider) responsesInput(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					args, _ := json.Marshal(tc.Args)
					out = append(out, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      tc.Name,
						"arguments": string(args),
					})
				}
			} else if m.Content != "" {
				out = append(out, map[string]any{
					"role": "assistant",
					"content": []map[string]any{
						{"type": "input_text", "text": m.Content},
					},
				})
			}
		case "tool":
			out = append(out, map[string]any{
				"type":   "function_call_output",
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		default:
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": m.Content},
				},
			})
		}
	}
	return out
}

func (p *OpenCodeProvider) responsesTools(tools []ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  params,
		})
	}
	return out
}

func (p *OpenCodeProvider) readResponsesSSE(ctx context.Context, r io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var data strings.Builder
	flush := func() bool {
		raw := strings.TrimSpace(data.String())
		data.Reset()
		if raw == "" || raw == "[DONE]" {
			return false
		}
		return p.handleResponsesEvent([]byte(raw), ch)
	}
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}
		line := scanner.Text()
		if line == "" {
			if flush() {
				return
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if data.Len() > 0 {
		_ = flush()
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("%s responses sse read: %w", p.name, err)}
	}
}

func (p *OpenCodeProvider) handleResponsesEvent(raw []byte, ch chan<- StreamEvent) bool {
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		return false
	}
	typ, _ := ev["type"].(string)
	switch typ {
	case "response.output_text.delta":
		if delta, _ := ev["delta"].(string); delta != "" {
			ch <- StreamEvent{TextDelta: delta}
		}
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		if delta, _ := ev["delta"].(string); delta != "" {
			ch <- StreamEvent{ReasoningDelta: delta}
		}
	case "response.function_call_arguments.delta":
		if args, _ := ev["arguments"].(string); args != "" {
			ch <- StreamEvent{ToolCallArgsDelta: args}
		}
	case "response.function_call_arguments.done":
		item, _ := ev["item"].(map[string]any)
		if item != nil && item["type"] == "function_call" {
			ch <- StreamEvent{ToolCallEnd: &ToolCall{
				Name: stringField(item, "name"),
				ID:   stringField(ev, "item_id"),
				Args: parseToolArgs(stringField(ev, "arguments")),
			}}
		}
	case "response.output_item.done":
		if item, _ := ev["item"].(map[string]any); item != nil && item["type"] == "function_call" {
			ch <- StreamEvent{ToolCallEnd: &ToolCall{
				Name: stringField(item, "name"),
				ID:   stringField(item, "call_id"),
				Args: parseToolArgs(stringField(item, "arguments")),
			}}
		}
	case "response.completed", "response.incomplete":
		if response, _ := ev["response"].(map[string]any); response != nil {
			if usage := parseResponsesUsage(response["usage"]); usage != nil {
				ch <- StreamEvent{Done: usage}
			}
		}
		return true
	case "response.failed":
		ch <- StreamEvent{Error: fmt.Errorf("%s responses stream: %s", p.name, strings.TrimSpace(string(raw)))}
		return true
	case "error":
		msg, _ := ev["message"].(string)
		ch <- StreamEvent{Error: fmt.Errorf("%s responses stream error: %s", p.name, msg)}
		return true
	}
	return false
}

func parseResponsesUsage(raw any) *Usage {
	m, _ := raw.(map[string]any)
	if m == nil {
		return nil
	}
	input := intNumber(m["input_tokens"])
	output := intNumber(m["output_tokens"])
	cached := 0
	if details, _ := m["input_tokens_details"].(map[string]any); details != nil {
		cached = intNumber(details["cached_tokens"])
	}
	return &Usage{InputTokens: input, OutputTokens: output, CacheReadTokens: cached}
}
