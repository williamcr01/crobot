package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"nhooyr.io/websocket"
)

const openAIResponsesWSURL = "wss://api.openai.com/v1/responses"

// OpenAIResponsesWSProvider uses the public OpenAI Responses API WebSocket
// transport. It keeps the latest response ID and sends incremental input on
// follow-up turns to benefit from the connection-local previous-response cache.
type OpenAIResponsesWSProvider struct {
	apiKey string
	url    string

	mu      sync.Mutex
	conn    *websocket.Conn
	prevID  string
	lastLen int
	closed  bool
}

func NewOpenAIResponsesWS(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai-responses-ws: missing API key")
	}
	return &OpenAIResponsesWSProvider{apiKey: apiKey, url: openAIResponsesWSURL}, nil
}

func (p *OpenAIResponsesWSProvider) Name() string { return "openai-responses-ws" }

func (p *OpenAIResponsesWSProvider) ListModels(ctx context.Context) ([]string, error) {
	base, err := NewOpenAI(p.apiKey)
	if err != nil {
		return nil, err
	}
	return base.ListModels(ctx)
}

func (p *OpenAIResponsesWSProvider) Send(ctx context.Context, req Request) (*Response, error) {
	stream, err := p.Stream(ctx, req)
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
}

func (p *OpenAIResponsesWSProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	body, expectedLen, err := p.nextCreateBody(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		if err := p.write(ctx, payload); err != nil {
			ch <- StreamEvent{Error: err}
			return
		}
		p.readLoop(ctx, ch, expectedLen)
	}()
	return ch, nil
}

func (p *OpenAIResponsesWSProvider) nextCreateBody(req Request) (map[string]any, int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	usePrevious := p.prevID != "" && len(req.Messages) >= p.lastLen
	inputMessages := req.Messages
	if usePrevious {
		inputMessages = incrementalResponsesMessages(req.Messages[p.lastLen:])
	} else {
		p.prevID = ""
		p.lastLen = 0
	}

	body := map[string]any{
		"type":                "response.create",
		"model":               req.Model,
		"store":               false,
		"instructions":        req.SystemPrompt,
		"input":               responsesInputItems(inputMessages),
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
		"text":                map[string]any{"verbosity": "low"},
	}
	if usePrevious {
		body["previous_response_id"] = p.prevID
	}
	if len(req.Tools) > 0 {
		body["tools"] = responsesToolItems(req.Tools)
	}
	if effort, ok := openAIReasoningEffort(req.Model, req.Thinking); ok && effort != "none" {
		body["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
	}
	return body, len(req.Messages), nil
}

func incrementalResponsesMessages(messages []Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		// Prior assistant outputs already live in the server-side previous response.
		if m.Role == "assistant" {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (p *OpenAIResponsesWSProvider) write(ctx context.Context, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("openai-responses-ws: provider closed")
	}
	if p.conn == nil {
		header := http.Header{}
		header.Set("Authorization", "Bearer "+p.apiKey)
		conn, _, err := websocket.Dial(ctx, p.url, &websocket.DialOptions{HTTPHeader: header})
		if err != nil {
			return fmt.Errorf("openai-responses-ws dial: %w", err)
		}
		p.conn = conn
	}
	if err := p.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		_ = p.conn.Close(websocket.StatusInternalError, "write failed")
		p.conn = nil
		return fmt.Errorf("openai-responses-ws write: %w", err)
	}
	return nil
}

func (p *OpenAIResponsesWSProvider) readLoop(ctx context.Context, ch chan<- StreamEvent, expectedLen int) {
	for {
		_, raw, err := p.read(ctx)
		if err != nil {
			ch <- StreamEvent{Error: err}
			return
		}
		stop, responseID := handleResponsesWSEvent(raw, ch)
		if responseID != "" {
			p.mu.Lock()
			p.prevID = responseID
			p.lastLen = expectedLen
			p.mu.Unlock()
		}
		if stop {
			return
		}
	}
}

func (p *OpenAIResponsesWSProvider) read(ctx context.Context) (websocket.MessageType, []byte, error) {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn == nil {
		return 0, nil, fmt.Errorf("openai-responses-ws: connection is not open")
	}
	typ, raw, err := conn.Read(ctx)
	if err != nil {
		p.mu.Lock()
		if p.conn == conn {
			p.conn = nil
			p.prevID = ""
			p.lastLen = 0
		}
		p.mu.Unlock()
		return 0, nil, fmt.Errorf("openai-responses-ws read: %w", err)
	}
	return typ, raw, nil
}

func handleResponsesWSEvent(raw []byte, ch chan<- StreamEvent) (stop bool, responseID string) {
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		return false, ""
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
	case "response.output_item.done":
		if item, _ := ev["item"].(map[string]any); item != nil && item["type"] == "function_call" {
			args, _ := item["arguments"].(string)
			ch <- StreamEvent{ToolCallEnd: &ToolCall{Name: stringField(item, "name"), ID: stringField(item, "call_id"), Args: parseToolArgs(args)}}
		}
	case "response.completed", "response.done", "response.incomplete":
		if response, _ := ev["response"].(map[string]any); response != nil {
			responseID = stringField(response, "id")
			if usage := parseCodexUsage(response["usage"]); usage != nil {
				ch <- StreamEvent{Done: usage}
			}
		}
		return true, responseID
	case "response.failed", "error":
		ch <- StreamEvent{Error: fmt.Errorf("openai-responses-ws stream: %s", strings.TrimSpace(string(raw)))}
		return true, ""
	}
	return false, ""
}

func responsesInputItems(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					args, _ := json.Marshal(tc.Args)
					out = append(out, map[string]any{"type": "function_call", "call_id": tc.ID, "name": tc.Name, "arguments": string(args)})
				}
			} else if m.Content != "" {
				out = append(out, map[string]any{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": m.Content}}})
			}
		case "tool":
			out = append(out, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content})
		default:
			out = append(out, map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": m.Content}}})
		}
	}
	return out
}

func responsesToolItems(tools []ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		}
		out = append(out, map[string]any{"type": "function", "name": t.Name, "description": t.Description, "parameters": params})
	}
	return out
}
