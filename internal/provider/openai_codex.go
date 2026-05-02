package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const openAICodexBaseURL = "https://chatgpt.com/backend-api/codex/responses"

// OpenAICodexProvider uses ChatGPT/Codex OAuth tokens against the ChatGPT
// backend Codex Responses endpoint. This is intentionally separate from the
// OpenAI API-key provider, which uses api.openai.com Chat Completions.
type OpenAICodexProvider struct {
	accessToken string
	accountID   string
	baseURL     string
	client      *http.Client
}

func NewOpenAICodex(apiKey string) (Provider, error) {
	accessToken := strings.TrimSpace(apiKey)
	if accessToken == "" {
		return nil, fmt.Errorf("openai-codex: missing OAuth access token")
	}
	accountID := openAICodexAccountID(accessToken)
	if accountID == "" {
		return nil, fmt.Errorf("openai-codex: failed to extract ChatGPT account ID from OAuth access token")
	}
	return &OpenAICodexProvider{
		accessToken: accessToken,
		accountID:   accountID,
		baseURL:     openAICodexBaseURL,
		client:      &http.Client{Timeout: 0},
	}, nil
}

func (p *OpenAICodexProvider) Name() string { return "openai-codex" }

func (p *OpenAICodexProvider) ListModels(context.Context) ([]string, error) {
	return openAIOAuthModels(), nil
}

func (p *OpenAICodexProvider) Send(ctx context.Context, req Request) (*Response, error) {
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

func (p *OpenAICodexProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	body, err := json.Marshal(p.toResponsesBody(req))
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.addHeaders(httpReq)

	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		resp, err := p.client.Do(httpReq)
		if err != nil {
			ch <- StreamEvent{Error: fmt.Errorf("openai-codex stream: %w", err)}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := io.ReadAll(resp.Body)
			ch <- StreamEvent{Error: fmt.Errorf("openai-codex stream: POST %q: %s %s", p.baseURL, resp.Status, strings.TrimSpace(string(raw)))}
			return
		}
		p.readSSE(ctx, resp.Body, ch)
	}()
	return ch, nil
}

func (p *OpenAICodexProvider) addHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.accessToken)
	req.Header.Set("chatgpt-account-id", p.accountID)
	req.Header.Set("originator", "crobot")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "crobot")
}

func (p *OpenAICodexProvider) toResponsesBody(req Request) map[string]any {
	body := map[string]any{
		"model":               req.Model,
		"store":               false,
		"stream":              true,
		"instructions":        req.SystemPrompt,
		"input":               p.responsesInput(req.Messages),
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
		"text":                map[string]any{"verbosity": "low"},
		"include":             []string{"reasoning.encrypted_content"},
	}
	if len(req.Tools) > 0 {
		body["tools"] = p.responsesTools(req.Tools)
	}
	if effort, ok := openAIReasoningEffort(req.Model, req.Thinking); ok && effort != "none" {
		body["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
	}
	return body
}

func (p *OpenAICodexProvider) responsesInput(messages []Message) []map[string]any {
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
				out = append(out, map[string]any{"role": "assistant", "content": []map[string]any{{"type": "output_text", "text": m.Content}}})
			}
		case "tool":
			out = append(out, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content})
		default:
			out = append(out, map[string]any{"role": "user", "content": []map[string]any{{"type": "input_text", "text": m.Content}}})
		}
	}
	return out
}

func (p *OpenAICodexProvider) responsesTools(tools []ToolDefinition) []map[string]any {
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

func (p *OpenAICodexProvider) readSSE(ctx context.Context, r io.Reader, ch chan<- StreamEvent) {
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
		ch <- StreamEvent{Error: fmt.Errorf("openai-codex stream read: %w", err)}
	}
}

func (p *OpenAICodexProvider) handleResponsesEvent(raw []byte, ch chan<- StreamEvent) bool {
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
	case "response.output_item.done":
		if item, _ := ev["item"].(map[string]any); item != nil && item["type"] == "function_call" {
			args, _ := item["arguments"].(string)
			ch <- StreamEvent{ToolCallEnd: &ToolCall{Name: stringField(item, "name"), ID: stringField(item, "call_id"), Args: parseToolArgs(args)}}
		}
	case "response.completed", "response.done", "response.incomplete":
		if response, _ := ev["response"].(map[string]any); response != nil {
			if usage := parseCodexUsage(response["usage"]); usage != nil {
				ch <- StreamEvent{Done: usage}
			}
		}
		return true
	case "response.failed", "error":
		ch <- StreamEvent{Error: fmt.Errorf("openai-codex stream: %s", strings.TrimSpace(string(raw)))}
		return true
	}
	return false
}

func parseCodexUsage(raw any) *Usage {
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
	return &Usage{InputTokens: input, OutputTokens: output, CacheReadTokens: cached, Subscription: true}
}

func intNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func openAICodexAccountID(accessToken string) string {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	accountID, _ := auth["chatgpt_account_id"].(string)
	return accountID
}
