package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func init() {
	Register("anthropic", NewAnthropic)
}

const anthropicDefaultMaxTokens int64 = 8192

// AnthropicProvider implements Provider using Anthropic's Messages API.
type AnthropicProvider struct {
	apiKey string
	client anthropic.Client
}

func NewAnthropic(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("anthropic: missing API key")
	}
	return &AnthropicProvider{
		apiKey: apiKey,
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Send(ctx context.Context, req Request) (*Response, error) {
	msg, err := p.client.Messages.New(ctx, p.toMessageParams(req))
	if err != nil {
		return nil, fmt.Errorf("anthropic send: %w", err)
	}
	return mapAnthropicMessage(msg), nil
}

func (p *AnthropicProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	stream := p.client.Messages.NewStreaming(ctx, p.toMessageParams(req))
	ch := make(chan StreamEvent, 16)
	go p.streamLoop(ctx, stream, ch)
	return ch, nil
}

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]string, error) {
	pager := p.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	var ids []string
	for pager.Next() {
		ids = append(ids, pager.Current().ID)
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("anthropic list models: %w", err)
	}
	return ids, nil
}

func (p *AnthropicProvider) toMessageParams(req Request) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		MaxTokens: anthropicDefaultMaxTokens,
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
		params.Tools = anthropicTools(req.Tools)
	}

	if thinking := anthropicThinking(req.Thinking); thinking != nil {
		params.Thinking = *thinking
	}

	return params
}

func anthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
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

func anthropicThinking(thinking string) *anthropic.ThinkingConfigParamUnion {
	var budget int64
	switch thinking {
	case "", "none":
		return nil
	case "minimal", "low":
		budget = 1024
	case "medium":
		budget = 2048
	case "high":
		budget = 4096
	case "xhigh":
		budget = 6144
	default:
		budget = 2048
	}
	cfg := anthropic.ThinkingConfigParamOfEnabled(budget)
	if cfg.OfEnabled != nil {
		cfg.OfEnabled.Display = anthropic.ThinkingConfigEnabledDisplaySummarized
	}
	return &cfg
}

type anthropicStream interface {
	Next() bool
	Current() anthropic.MessageStreamEventUnion
	Err() error
}

func (p *AnthropicProvider) streamLoop(ctx context.Context, stream anthropicStream, ch chan<- StreamEvent) {
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
			usage = &Usage{InputTokens: int(ev.Message.Usage.InputTokens), OutputTokens: int(ev.Message.Usage.OutputTokens)}
		case anthropic.MessageDeltaEvent:
			if usage == nil {
				usage = &Usage{}
			}
			usage.InputTokens = int(ev.Usage.InputTokens)
			usage.OutputTokens = int(ev.Usage.OutputTokens)
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
		ch <- StreamEvent{Error: fmt.Errorf("anthropic stream: %w", err)}
		return
	}
	if usage != nil {
		ch <- StreamEvent{Done: usage}
	}
}

func mapAnthropicMessage(msg *anthropic.Message) *Response {
	out := &Response{}
	if msg == nil {
		return out
	}
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Text += b.Text
		case anthropic.ThinkingBlock:
			// Non-streaming callers do not have a reasoning channel; keep the text
			// response focused on visible assistant output.
		case anthropic.ToolUseBlock:
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				Name: b.Name,
				ID:   b.ID,
				Args: parseToolArgs(string(b.Input)),
			})
		}
	}
	if msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0 {
		out.Usage = &Usage{InputTokens: int(msg.Usage.InputTokens), OutputTokens: int(msg.Usage.OutputTokens)}
	}
	return out
}
