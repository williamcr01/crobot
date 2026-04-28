package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"crobot/internal/provider"
	"crobot/internal/tools"
)

// Event is emitted by the agent runner as it streams.
type Event struct {
	// Type identifies which fields are set.
	Type string

	// Message events.
	MessageStart *MessageStartEvent
	MessageEnd   *MessageEndEvent

	// Text streaming.
	TextDelta      string
	ReasoningDelta string

	// Tool call events.
	ToolCallStart *ToolCallEvent
	ToolCallEnd   *ToolCallEvent
	ToolCallArgs  string

	// Tool execution events.
	ToolExecStart  *ToolExecStartEvent
	ToolExecResult *ToolExecResultEvent

	// Turn lifecycle.
	TurnStart string // turn ID
	TurnEnd   string // turn ID (empty = last)

	// Error.
	Error error
}

// MessageStartEvent is emitted when a new message begins.
type MessageStartEvent struct {
	Role    string // "user", "assistant", "tool"
	Content string // full content of the message (for user messages)
}

// MessageEndEvent is emitted when a message is finalized.
type MessageEndEvent struct {
	Role  string
	Text  string // accumulated text for assistant messages
	Usage *provider.Usage
}

// ToolCallEvent describes a tool call from the model.
type ToolCallEvent struct {
	Name   string
	CallID string
	Args   map[string]any
}

// ToolExecStartEvent is emitted before a tool runs.
type ToolExecStartEvent struct {
	Name   string
	CallID string
	Args   map[string]any
}

// ToolExecResultEvent is emitted after a tool completes.
type ToolExecResultEvent struct {
	Name     string
	CallID   string
	Output   string
	Success  bool
	Duration int64 // milliseconds
}

// Result holds the final state of an agent run.
type Result struct {
	Text  string
	Usage *provider.Usage
}

// PluginManager is the subset of plugin.Manager used by the runner.
type PluginManager interface {
	CallPrePrompt(systemPrompt string, messages []provider.Message) (string, []provider.Message, error)
	CallPostResponse(resp *Result) (*Result, error)
	CallPreToolCall(name string, args map[string]any) (string, map[string]any, bool, error)
	CallPostToolResult(name string, args map[string]any, result any) (any, error)
	CallOnEvent(event any) error
}

// Run executes the agent loop: send messages to the provider, stream events,
// execute tool calls, repeat until no more tool calls.
func Run(
	ctx context.Context,
	prov provider.Provider,
	model string,
	systemPrompt string,
	messages []provider.Message,
	toolReg *tools.Registry,
	plugins PluginManager,
	onEvent func(Event),
) (*Result, error) {
	return RunWithThinking(ctx, prov, model, "none", 50, systemPrompt, messages, toolReg, plugins, onEvent)
}

// RunWithThinking executes the agent loop with an explicit thinking effort.
func RunWithThinking(
	ctx context.Context,
	prov provider.Provider,
	model string,
	thinking string,
	maxTurns int,
	systemPrompt string,
	messages []provider.Message,
	toolReg *tools.Registry,
	plugins PluginManager,
	onEvent func(Event),
) (*Result, error) {
	r := &runner{
		prov:         prov,
		model:        model,
		thinking:     thinking,
		maxTurns:     maxTurns,
		systemPrompt: systemPrompt,
		toolReg:      toolReg,
		plugins:      plugins,
		onEvent:      onEvent,
		messages:     make([]provider.Message, len(messages)),
	}
	copy(r.messages, messages)
	return r.run(ctx)
}

type runner struct {
	prov         provider.Provider
	model        string
	thinking     string
	maxTurns     int
	systemPrompt string
	toolReg      *tools.Registry
	plugins      PluginManager
	onEvent      func(Event)

	messages []provider.Message
}

func (r *runner) run(ctx context.Context) (*Result, error) {
	for turn := 0; ; turn++ {
		if r.maxTurns >= 0 && turn >= r.maxTurns {
			err := fmt.Errorf("agent stopped after %d turns to prevent an infinite loop", r.maxTurns)
			r.emit(Event{Type: "error", Error: err})
			return nil, err
		}
		// Plugin hook: pre-prompt.
		sysPrompt := r.systemPrompt
		msgs := r.messages
		if r.plugins != nil {
			var err error
			sysPrompt, msgs, err = r.plugins.CallPrePrompt(sysPrompt, msgs)
			if err != nil {
				return nil, fmt.Errorf("plugin pre_prompt: %w", err)
			}
		}

		// Build request.
		req := provider.Request{
			Model:        r.model,
			Thinking:     r.thinking,
			SystemPrompt: sysPrompt,
			Messages:     msgs,
			Tools:        r.toolReg.ToProviderTools(),
			Stream:       true,
		}

		// Emit turn start.
		r.emit(Event{Type: "turn_start"})

		// Stream with retry.
		step, err := r.streamWithRetry(ctx, req)
		if err != nil {
			r.emit(Event{Type: "error", Error: err})
			return nil, err
		}

		// Add assistant message, preserving tool-call metadata for the follow-up request.
		r.messages = append(r.messages, provider.Message{
			Role:      "assistant",
			Content:   step.Text,
			ToolCalls: step.ToolCalls,
		})

		// If there are NO tool calls, we're done.
		if len(step.ToolCalls) == 0 {
			r.emit(Event{Type: "turn_end"})
			r.emit(Event{
				Type: "message_end",
				MessageEnd: &MessageEndEvent{
					Role:  "assistant",
					Text:  step.Text,
					Usage: step.Usage,
				},
			})

			result := &Result{Text: step.Text, Usage: step.Usage}
			if r.plugins != nil {
				var err error
				result, err = r.plugins.CallPostResponse(result)
				if err != nil {
					return nil, fmt.Errorf("plugin post_response: %w", err)
				}
			}
			return result, nil
		}

		// Execute tool calls.
		for _, tc := range step.ToolCalls {
			r.emit(Event{
				Type: "tool_exec_start",
				ToolExecStart: &ToolExecStartEvent{
					Name:   tc.Name,
					CallID: tc.ID,
					Args:   tc.Args,
				},
			})

			output, success, dur := r.executeTool(ctx, tc)

			r.emit(Event{
				Type: "tool_exec_result",
				ToolExecResult: &ToolExecResultEvent{
					Name:     tc.Name,
					CallID:   tc.ID,
					Output:   output,
					Success:  success,
					Duration: dur.Milliseconds(),
				},
			})

			// Add tool result message with the matching tool call ID.
			r.messages = append(r.messages, provider.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    output,
			})
		}

		r.emit(Event{Type: "turn_end"})
	}
}

type streamStep struct {
	Text      string
	ToolCalls []provider.ToolCall
	Usage     *provider.Usage
}

func (r *runner) streamWithRetry(ctx context.Context, req provider.Request) (*streamStep, error) {
	var lastErr error
	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(min(1000*int64(1<<(attempt-1)), 30000)) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		evCh, err := r.prov.Stream(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}

		return r.processStream(ctx, evCh)
	}
	return nil, lastErr
}

func (r *runner) processStream(ctx context.Context, evCh <-chan provider.StreamEvent) (*streamStep, error) {
	type toolCallAccum struct {
		name    string
		id      string
		argsBuf strings.Builder
	}

	var (
		currentText      strings.Builder
		toolCalls        []provider.ToolCall
		usage            *provider.Usage
		toolCallsPending = make(map[string]*toolCallAccum)
		currentCallID    string
	)

	r.emit(Event{
		Type: "message_start",
		MessageStart: &MessageStartEvent{
			Role: "assistant",
		},
	})

	for event := range evCh {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if event.Error != nil {
			return nil, event.Error
		}

		if event.TextDelta != "" {
			currentText.WriteString(event.TextDelta)
			r.emit(Event{Type: "text_delta", TextDelta: event.TextDelta})
		}

		if event.ReasoningDelta != "" {
			r.emit(Event{Type: "reasoning_delta", ReasoningDelta: event.ReasoningDelta})
		}

		if event.ToolCallStart != nil {
			tc := event.ToolCallStart
			toolCallsPending[tc.ID] = &toolCallAccum{
				name: tc.Name,
				id:   tc.ID,
			}
			currentCallID = tc.ID
			r.emit(Event{
				Type: "tool_call_start",
				ToolCallStart: &ToolCallEvent{
					Name:   tc.Name,
					CallID: tc.ID,
				},
			})
		}

		if event.ToolCallArgsDelta != "" {
			// Accumulate into the active tool call (last started).
			if acc, ok := toolCallsPending[currentCallID]; ok {
				acc.argsBuf.WriteString(event.ToolCallArgsDelta)
				r.emit(Event{Type: "tool_call_args", ToolCallArgs: event.ToolCallArgsDelta})
			}
		}

		if event.ToolCallEnd != nil {
			tc := event.ToolCallEnd
			if acc, ok := toolCallsPending[tc.ID]; ok {
				var args map[string]any
				if len(tc.Args) > 0 {
					args = tc.Args
				} else {
					args = parseArgs(acc.argsBuf.String())
				}
				toolCalls = append(toolCalls, provider.ToolCall{
					Name: tc.Name,
					ID:   acc.id,
					Args: args,
				})
				delete(toolCallsPending, tc.ID)
				r.emit(Event{
					Type: "tool_call_end",
					ToolCallEnd: &ToolCallEvent{
						Name:   tc.Name,
						CallID: acc.id,
						Args:   args,
					},
				})
			}
		}

		if event.Done != nil {
			usage = event.Done
		}
	}

	text := currentText.String()

	return &streamStep{
		Text:      text,
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}

func (r *runner) executeTool(ctx context.Context, tc provider.ToolCall) (output string, success bool, duration time.Duration) {
	start := time.Now()
	name := tc.Name
	args := tc.Args

	if r.plugins != nil {
		var skip bool
		var err error
		name, args, skip, err = r.plugins.CallPreToolCall(name, args)
		if err != nil {
			return fmt.Sprintf("tool call blocked: %v", err), false, time.Since(start)
		}
		if skip {
			return "tool call skipped by plugin", false, time.Since(start)
		}
	}

	rawResult, err := r.toolReg.Execute(ctx, name, args)
	dur := time.Since(start)

	if err != nil {
		return fmt.Sprintf("error: %v", err), false, dur
	}

	var result string
	switch v := rawResult.(type) {
	case map[string]any:
		if err, ok := v["error"].(string); ok {
			return err, false, dur
		}
		result = fmt.Sprintf("%v", rawResult)
	case string:
		result = v
	default:
		result = fmt.Sprintf("%v", rawResult)
	}

	if r.plugins != nil {
		modified, hookErr := r.plugins.CallPostToolResult(name, args, rawResult)
		if hookErr == nil && modified != nil {
			result = fmt.Sprintf("%v[plugin modified]", modified)
		}
	}

	return result, true, dur
}

func (r *runner) emit(ev Event) {
	if r.onEvent != nil {
		r.onEvent(ev)
	}
}

func parseArgs(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var args map[string]any
	if err := provider.ParseJSON(raw, &args); err == nil {
		return args
	}
	return map[string]any{"raw": raw}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
