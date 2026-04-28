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
)

func init() {
	Register("openai", NewOpenAI)
	Register("openai-oauth", NewOpenAIOAuth)
}

const openAIBaseURL = "https://api.openai.com/v1"

type OpenAIProvider struct {
	name   string
	apiKey string
	client *http.Client
}

func NewOpenAI(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai: missing API key or OAuth access token")
	}
	return &OpenAIProvider{name: "openai", apiKey: apiKey, client: http.DefaultClient}, nil
}

func NewOpenAIOAuth(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai-oauth: missing OAuth access token")
	}
	return &OpenAIProvider{name: "openai-oauth", apiKey: apiKey, client: http.DefaultClient}, nil
}

func (p *OpenAIProvider) Name() string { return p.name }

func (p *OpenAIProvider) Send(ctx context.Context, req Request) (*Response, error) {
	body := p.buildChatRequest(req, false)
	var res openAIChatResponse
	if err := p.doJSON(ctx, http.MethodPost, "/chat/completions", body, &res); err != nil {
		return nil, err
	}
	return mapOpenAIResponse(&res), nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	body := p.buildChatRequest(req, true)
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	p.setHeaders(hreq)
	resp, err := p.client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("openai stream: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("openai stream: %s: %s", resp.Status, readErrorBody(resp.Body))
	}
	ch := make(chan StreamEvent, 16)
	go p.streamLoop(ctx, resp.Body, ch)
	return ch, nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	if p.name == "openai-oauth" || isOpenAIOAuthToken(p.apiKey) {
		return openAIOAuthModels(), nil
	}

	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := p.doJSON(ctx, http.MethodGet, "/models", nil, &res); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res.Data))
	for _, m := range res.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
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

func (p *OpenAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
}

func (p *OpenAIProvider) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, openAIBaseURL+path, r)
	if err != nil {
		return err
	}
	p.setHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openai %s: %s: %s", path, resp.Status, readErrorBody(resp.Body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *OpenAIProvider) buildChatRequest(req Request, stream bool) map[string]any {
	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			msg["tool_calls"] = toOpenAIToolCalls(m.ToolCalls)
		}
		if m.Role == "tool" {
			msg["tool_call_id"] = m.ToolCallID
		}
		messages = append(messages, msg)
	}
	body := map[string]any{"model": req.Model, "messages": messages, "stream": stream}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			params := t.InputSchema
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
			}
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": params}})
		}
		body["tools"] = tools
	}
	if effort, ok := openAIReasoningEffort(req.Model, req.Thinking); ok {
		body["reasoning_effort"] = effort
	}
	return body
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

func toOpenAIToolCalls(calls []ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, tc := range calls {
		args, _ := json.Marshal(tc.Args)
		out = append(out, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": string(args)}})
	}
	return out
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
type openAIToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func mapOpenAIResponse(res *openAIChatResponse) *Response {
	out := &Response{}
	if len(res.Choices) > 0 {
		msg := res.Choices[0].Message
		out.Text = msg.Content
		for _, tc := range msg.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{Name: tc.Function.Name, ID: tc.ID, Args: parseToolArgs(tc.Function.Arguments)})
		}
	}
	if res.Usage != nil {
		out.Usage = &Usage{InputTokens: res.Usage.PromptTokens, OutputTokens: res.Usage.CompletionTokens}
	}
	return out
}

func (p *OpenAIProvider) streamLoop(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	type pendingTool struct {
		name string
		id   string
		args strings.Builder
	}
	pending := map[int]*pendingTool{}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- StreamEvent{Error: err}
			continue
		}
		if chunk.Error != nil {
			ch <- StreamEvent{Error: fmt.Errorf("openai: %s", chunk.Error.Message)}
			continue
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- StreamEvent{TextDelta: choice.Delta.Content}
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
			if choice.FinishReason != nil {
				for _, pnd := range pending {
					if pnd.name != "" {
						ch <- StreamEvent{ToolCallEnd: &ToolCall{Name: pnd.name, ID: pnd.id, Args: parseToolArgs(pnd.args.String())}}
					}
				}
				pending = map[int]*pendingTool{}
			}
		}
		if chunk.Usage != nil {
			ch <- StreamEvent{Done: &Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("openai stream: %w", err)}
	}
}

func readErrorBody(r io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(r, 4096))
	return strings.TrimSpace(string(data))
}
