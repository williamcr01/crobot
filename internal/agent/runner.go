package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"crobot/internal/config"
	"crobot/internal/provider"
	"crobot/internal/tools"
)

// Event is emitted by the agent runner as it streams.
type Event struct {
	Text        string         // Text delta
	Reasoning   string         // Reasoning delta
	ToolCall    *ToolCallEvent // Tool call started or finished
	ToolArgs    string         // Tool args delta (JSON fragment)
	ToolResult  *ToolCallResult
	Usage       *provider.Usage
	Error       error
}

// ToolCallEvent is emitted when the model makes a tool call.
type ToolCallEvent struct {
	Name   string
	CallID string
	Args   map[string]any
	Start  bool // true for start, false for end
}

// ToolCallResult is emitted after a tool finishes executing.
type ToolCallResult struct {
	Name    string
	CallID  string
	Output  string
	Success bool
}

// Result holds the final output of an agent run.
type Result struct {
	Text      string
	ToolCalls []provider.ToolCall
	Usage     *provider.Usage
}

// PluginManager is the subset of plugin.Manager used by the runner.
// This avoids a circular import between agent and plugins.
type PluginManager interface {
	CallPrePrompt(systemPrompt string, messages []provider.Message) (string, []provider.Message, error)
	CallPostResponse(resp *Result) (*Result, error)
	CallPreToolCall(name string, args map[string]any) (string, map[string]any, bool, error)
	CallPostToolResult(name string, args map[string]any, result any) (any, error)
	CallOnEvent(event any) error
}

// Run executes the agent loop: send messages to the provider, stream events,
// execute tool calls, repeat until stop conditions are met.
func Run(
	ctx context.Context,
	prov provider.Provider,
	cfg *config.AgentConfig,
	history []provider.Message,
	toolReg *tools.Registry,
	plugins PluginManager,
	onEvent func(Event),
) (*Result, error) {
	runner := &runner{
		prov:     prov,
		cfg:      cfg,
		toolReg:  toolReg,
		plugins:  plugins,
		onEvent:  onEvent,
		stepCount: 0,
		totalCost: 0,
	}
	return runner.run(ctx, history)
}

type runner struct {
	prov      provider.Provider
	cfg       *config.AgentConfig
	toolReg   *tools.Registry
	plugins   PluginManager
	onEvent   func(Event)

	messages  []provider.Message
	stepCount int
	totalCost float64
}

func (r *runner) run(ctx context.Context, history []provider.Message) (*Result, error) {
	r.messages = history

	currentText := ""
	currentToolCalls := make(map[string]*provider.ToolCall) // callID -> partial

	for {
		// Stop conditions.
		if r.stepCount >= r.cfg.MaxSteps {
			return r.makeResult(currentText, currentToolCalls), nil
		}
		if r.totalCost >= r.cfg.MaxCost {
			return r.makeResult(currentText, currentToolCalls), nil
		}

		// Plugin hook: pre-prompt.
		systemPrompt := r.cfg.SystemPrompt
		if r.plugins != nil {
			var err error
			systemPrompt, r.messages, err = r.plugins.CallPrePrompt(systemPrompt, r.messages)
			if err != nil {
				return nil, fmt.Errorf("plugin pre_prompt: %w", err)
			}
		}

		// Build request.
		req := provider.Request{
			Model:        r.cfg.Model,
			SystemPrompt: systemPrompt,
			Messages:     r.messages,
			Tools:        r.toolReg.ToProviderTools(),
			Stream:       true,
		}

		// Retry loop.
		var lastErr error
		for attempt := 0; attempt <= 3; attempt++ {
			if attempt > 0 {
				backoff := time.Duration(math.Min(1000*math.Pow(2, float64(attempt-1)), 30000)) * time.Millisecond
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

			stepResult, err := r.processStream(ctx, evCh, &currentText, currentToolCalls)
			if err != nil {
				lastErr = err
				continue
			}

			r.stepCount++
			r.totalCost += estimateCost(stepResult.Usage, r.cfg.Model)

			// If there are tool calls to execute, append results and loop.
			if len(stepResult.ToolCalls) > 0 {
				// Append assistant message with tool calls.
				assistantContent := stepResult.Text
				r.messages = append(r.messages, provider.Message{
					Role:    "assistant",
					Content: assistantContent,
				})

				// Execute each tool call.
				for _, tc := range stepResult.ToolCalls {
					toolResult := r.executeTool(ctx, tc)
					r.messages = append(r.messages, provider.Message{
						Role:    "tool",
						Content: fmt.Sprintf("%s: %s", tc.ID, toolResult),
					})
				}

				// Reset for next iteration.
				currentText = ""
				currentToolCalls = make(map[string]*provider.ToolCall)
				lastErr = nil
				break // exit retry loop, continue agent loop
			}

			// No tool calls, done.
			// Plugin hook: post-response.
			result := r.makeResult(stepResult.Text, currentToolCalls)
			result.Usage = stepResult.Usage
			if r.plugins != nil {
				var err error
				result, err = r.plugins.CallPostResponse(result)
				if err != nil {
					return nil, fmt.Errorf("plugin post_response: %w", err)
				}
			}
			return result, nil
		}

		if lastErr != nil {
			return nil, lastErr
		}
	}
}

type stepResult struct {
	Text      string
	ToolCalls []provider.ToolCall
	Usage     *provider.Usage
}

func (r *runner) processStream(
	ctx context.Context,
	evCh <-chan provider.StreamEvent,
	currentText *string,
	currentToolCalls map[string]*provider.ToolCall,
) (*stepResult, error) {
	var toolCalls []provider.ToolCall
	var usage *provider.Usage

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
			*currentText += event.TextDelta
			r.emit(Event{Text: event.TextDelta})
		}

		if event.ReasoningDelta != "" {
			r.emit(Event{Reasoning: event.ReasoningDelta})
		}

		if event.ToolCallStart != nil {
			currentToolCalls[event.ToolCallStart.ID] = event.ToolCallStart
			r.emit(Event{ToolCall: &ToolCallEvent{
				Name:   event.ToolCallStart.Name,
				CallID: event.ToolCallStart.ID,
				Start:  true,
			}})
		}

		if event.ToolCallArgsDelta != "" {
			// Find the current in-progress tool call and update it.
			for _, tc := range currentToolCalls {
				if tc.Args == nil {
					tc.Args = make(map[string]any)
				}
				// We can't easily associate args deltas with a specific call ID
				// in the SDK stream. For now, we emit the delta.
			}
			r.emit(Event{ToolArgs: event.ToolCallArgsDelta})
		}

		if event.ToolCallEnd != nil {
			currentToolCalls[event.ToolCallEnd.ID] = event.ToolCallEnd
			toolCalls = append(toolCalls, *event.ToolCallEnd)
			r.emit(Event{ToolCall: &ToolCallEvent{
				Name:   event.ToolCallEnd.Name,
				CallID: event.ToolCallEnd.ID,
				Args:   event.ToolCallEnd.Args,
				Start:  false,
			}})
		}

		if event.Done != nil {
			usage = event.Done
		}
	}

	return &stepResult{
		Text:      *currentText,
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}

func (r *runner) executeTool(ctx context.Context, tc provider.ToolCall) string {
	start := time.Now()

	name := tc.Name
	args := tc.Args
	skip := false

	// Plugin hook: pre-tool-call.
	if r.plugins != nil {
		var err error
		name, args, skip, err = r.plugins.CallPreToolCall(name, args)
		if err != nil {
			result := fmt.Sprintf("tool call blocked: %v", err)
			r.emit(Event{ToolResult: &ToolCallResult{
				Name:    tc.Name,
				CallID:  tc.ID,
				Output:  result,
				Success: false,
			}})
			return result
		}
	}

	if skip {
		result := "tool call skipped by plugin"
		r.emit(Event{ToolResult: &ToolCallResult{
			Name:    tc.Name,
			CallID:  tc.ID,
			Output:  result,
			Success: false,
		}})
		return result
	}

	rawResult, err := r.toolReg.Execute(ctx, name, args)
	result := formatToolResult(rawResult, err, time.Since(start))

	// Plugin hook: post-tool-result.
	if r.plugins != nil {
		modified, hookErr := r.plugins.CallPostToolResult(name, args, rawResult)
		if hookErr == nil && modified != nil {
			rawResult = modified
			result = formatToolResult(rawResult, nil, time.Since(start))
		}
	}

	r.emit(Event{ToolResult: &ToolCallResult{
		Name:    tc.Name,
		CallID:  tc.ID,
		Output:  result,
		Success: err == nil,
	}})

	return result
}

func (r *runner) emit(ev Event) {
	if r.plugins != nil {
		_ = r.plugins.CallOnEvent(ev)
	}
	if r.onEvent != nil {
		r.onEvent(ev)
	}
}

func (r *runner) makeResult(text string, toolCalls map[string]*provider.ToolCall) *Result {
	var tcs []provider.ToolCall
	for _, tc := range toolCalls {
		tcs = append(tcs, *tc)
	}
	return &Result{
		Text:      text,
		ToolCalls: tcs,
	}
}

func formatToolResult(rawResult any, err error, duration time.Duration) string {
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return fmt.Sprintf("%v", rawResult)
}

// estimateCost estimates the USD cost of a model response based on token usage.
// These are rough estimates for common models.
func estimateCost(usage *provider.Usage, model string) float64 {
	if usage == nil {
		return 0
	}
	// Average pricing per 1M tokens (input + output blended).
	// Claude Opus: ~$15/M input, ~$75/M output. Use conservative ~$30/M blended.
	ratePerMillion := 30.0
	inputTokens := float64(usage.InputTokens)
	outputTokens := float64(usage.OutputTokens)
	return ((inputTokens + outputTokens) / 1_000_000) * ratePerMillion
}
