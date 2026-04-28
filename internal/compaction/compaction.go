// Package compaction provides context compaction for long agent sessions.
//
// When the conversation context exceeds a threshold, older messages are
// summarized via the LLM and replaced with a compact summary, freeing
// room for the model to continue working.
package compaction

import (
	"context"
	"fmt"
	"math"
	"strings"

	"crobot/internal/config"
	"crobot/internal/provider"
)

// MessageItem mirrors tui.MessageItem for cut-point detection.
// Exported so the TUI can convert to/from it.
type MessageItem struct {
	Role      string // "user", "assistant", "system", "error", "compaction"
	Content   string
	Reasoning string
	ToolCalls []ToolRenderItem
}

// ToolRenderItem holds rendered state for one tool call.
type ToolRenderItem struct {
	Name    string
	CallID  string
	Output  string
	Args    string
	RawArgs map[string]any
}

const (
	roleUser      = "user"
	roleAssistant = "assistant"
	roleSystem    = "system"
)

// CanCompact checks whether there is enough content to compact.
// Returns false if the session is empty or already compacted.
func CanCompact(messages []MessageItem) bool {
	if len(messages) == 0 {
		return false
	}
	// Can't compact if the last message is already a compaction summary.
	if messages[len(messages)-1].Role == "compaction" {
		return false
	}
	return true
}

// estimateTokens returns a rough token count for a message using chars/4.
func estimateTokens(msg MessageItem) int {
	chars := len(msg.Content) + len(msg.Reasoning)
	for _, tc := range msg.ToolCalls {
		chars += len(tc.Args) + len(tc.Output)
	}
	return int(math.Ceil(float64(chars) / 4.0))
}

// findCutPoint walks backward from the newest message accumulating token
// estimates until keepTokens is reached, then returns the index of the
// first message to keep. Returns 0 if nothing to cut.
func findCutPoint(messages []MessageItem, keepTokens int) int {
	if len(messages) == 0 {
		return 0
	}

	accumulated := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == roleSystem {
			// System messages are kept (context). Don't count them.
			continue
		}
		accumulated += estimateTokens(msg)
		if accumulated >= keepTokens {
			// Walk forward to find the nearest user message boundary
			for j := i; j < len(messages); j++ {
				if messages[j].Role == roleUser {
					return j
				}
			}
			return i
		}
	}

	// Not enough content to compact.
	return -1
}

// needsCompaction checks whether compaction should trigger based on a token
// estimate and configured thresholds.
func needsCompaction(totalTokens int, settings config.CompactionConfig) bool {
	if !settings.Enabled {
		return false
	}
	// Estimate context window conservatively at 128k for most models.
	// The actual window depends on the model, but this catches oversized contexts.
	const contextWindow = 128 * 1024
	return totalTokens > contextWindow-settings.ReserveTokens
}

// estimateContextTokens returns a total token estimate for the message list.
func estimateContextTokens(messages []MessageItem) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokens(msg)
	}
	return total
}

// serializeMessages converts messages to a plain-text format for the LLM summarizer.
// Tool results are truncated to 2000 chars to keep summarization requests manageable.
func serializeMessages(messages []MessageItem) string {
	var b strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case roleUser:
			b.WriteString("[User]: ")
			b.WriteString(msg.Content)
			b.WriteString("\n")
		case roleAssistant:
			if msg.Reasoning != "" {
				b.WriteString("[Assistant thinking]: ")
				b.WriteString(msg.Reasoning)
				b.WriteString("\n")
			}
			b.WriteString("[Assistant]: ")
			b.WriteString(msg.Content)
			b.WriteString("\n")
			for _, tc := range msg.ToolCalls {
				b.WriteString(fmt.Sprintf("[Assistant tool call]: %s(%s)\n", tc.CallID, tc.Args))
			}
		case "tool":
			output := msg.Content
			if len(output) > 2000 {
				output = output[:2000] + fmt.Sprintf("\n... (%d chars truncated)", len(msg.Content)-2000)
			}
			b.WriteString("[Tool result]: ")
			b.WriteString(output)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// summarizedMessage creates a compaction summary MessageItem that acts as
// a context replacement for the summarized messages.
func summarizedMessage(summary string, tokensBefore int) MessageItem {
	content := fmt.Sprintf("[Context compacted — %d tokens summarized]\n\n%s", tokensBefore, summary)
	return MessageItem{
		Role:    "compaction",
		Content: content,
	}
}

// buildMessagesForAgent converts MessageItems to provider.Messages for the
// agent runner. This mirrors the logic in tui/model.go startAgent().
func buildMessagesForAgent(messages []MessageItem) []provider.Message {
	var llmMsgs []provider.Message
	for _, msg := range messages {
		switch msg.Role {
		case roleUser, roleSystem, "compaction":
			role := msg.Role
			if role == "compaction" {
				role = roleSystem
			}
			llmMsgs = append(llmMsgs, provider.Message{Role: role, Content: msg.Content})
		case roleAssistant:
			llmMsg := provider.Message{Role: "assistant", Content: msg.Content}
			for _, tc := range msg.ToolCalls {
				if tc.CallID != "" {
					llmMsg.ToolCalls = append(llmMsg.ToolCalls, provider.ToolCall{
						Name: tc.Name,
						ID:   tc.CallID,
						Args: tc.RawArgs,
					})
				}
			}
			llmMsgs = append(llmMsgs, llmMsg)
			for _, tc := range msg.ToolCalls {
				if tc.Output != "" {
					llmMsgs = append(llmMsgs, provider.Message{
						Role:       "tool",
						ToolCallID: tc.CallID,
						Content:    tc.Output,
					})
				}
			}
		}
	}
	return llmMsgs
}

const summarizationSystemPrompt = `You are a context summarizer. Your job is to create structured, concise summaries of coding agent conversations. These summaries will replace the full conversation in the agent's context window so the agent can continue working efficiently.

Be thorough about file paths, function names, code snippets, error messages, and technical decisions. The agent needs enough detail to continue seamlessly.`

const summarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another coding agent will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Any data, file paths, examples, or references needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

const updateSummarizationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.

Update the existing structured summary with new information. RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context from the new messages
- UPDATE the Progress section: move items from "In Progress" to "Done" when completed
- UPDATE "Next Steps" based on what was accomplished
- PRESERVE exact file paths, function names, and error messages
- If something is no longer relevant, you may remove it

Use this EXACT format:

## Goal
[Preserve existing goals, add new ones if the task expanded]

## Constraints & Preferences
- [Preserve existing, add new ones discovered]

## Progress
### Done
- [x] [Include previously done items AND newly completed items]

### In Progress
- [ ] [Current work - update based on progress]

### Blocked
- [Current blockers - remove if resolved]

## Key Decisions
- **[Decision]**: [Brief rationale] (preserve all previous, add new)

## Next Steps
1. [Update based on current state]

## Critical Context
- [Preserve important context, add new if needed]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

// generateSummary calls the LLM to produce a compaction summary from messages.
func generateSummary(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	messages []MessageItem,
	customInstructions string,
) (string, error) {
	conversationText := serializeMessages(messages)

	// Use initial summarization prompt (no previous summary support yet).
	prompt := summarizationPrompt
	if customInstructions != "" {
		prompt += "\n\nAdditional focus: " + customInstructions
	}

	promptText := "<conversation>\n" + conversationText + "\n</conversation>\n\n" + prompt

	req := provider.Request{
		Model:        modelID,
		SystemPrompt: summarizationSystemPrompt,
		Messages: []provider.Message{
			{Role: roleUser, Content: promptText},
		},
		Stream: false,
	}

	resp, err := prov.Send(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarization call failed: %w", err)
	}

	return resp.Text, nil
}

// Result holds the output of a compaction operation.
type Result struct {
	Summary       string
	TokensBefore  int
	KeptMessages  []MessageItem
	NewMessages   []MessageItem // replacement messages for the TUI
}

// Compact performs context compaction on the given messages.
//
// It estimates token usage, finds a cut point preserving keepRecentTokens,
// calls the LLM to summarize older messages, and returns the compacted
// message list.
//
// The provider is used for summarization. If compactionModel is non-empty,
// it overrides the model used for summarization.
func Compact(
	ctx context.Context,
	prov provider.Provider,
	model string,
	settings config.CompactionConfig,
	messages []MessageItem,
	customInstructions string,
) (*Result, error) {
	if !CanCompact(messages) {
		return nil, fmt.Errorf("nothing to compact")
	}

	tokensBefore := estimateContextTokens(messages)

	cutIndex := findCutPoint(messages, settings.KeepRecentTokens)
	if cutIndex < 0 {
		return nil, fmt.Errorf("session too small to compact (%d tokens)", tokensBefore)
	}
	if cutIndex == 0 {
		return nil, fmt.Errorf("nothing to summarize — all messages are recent")
	}

	messagesToSummarize := make([]MessageItem, cutIndex)
	copy(messagesToSummarize, messages[:cutIndex])
	keptMessages := make([]MessageItem, len(messages)-cutIndex)
	copy(keptMessages, messages[cutIndex:])

	summarizationModel := model
	if settings.Model != "" {
		summarizationModel = settings.Model
	}

	summary, err := generateSummary(ctx, prov, summarizationModel, messagesToSummarize, customInstructions)
	if err != nil {
		return nil, err
	}

	compactionMsg := summarizedMessage(summary, tokensBefore)
	newMessages := append([]MessageItem{compactionMsg}, keptMessages...)

	return &Result{
		Summary:      summary,
		TokensBefore: tokensBefore,
		KeptMessages: keptMessages,
		NewMessages:  newMessages,
	}, nil
}

// ShouldCompact checks if auto-compaction should trigger based on the
// estimated token count and settings.
func ShouldCompact(messages []MessageItem, settings config.CompactionConfig) bool {
	if !settings.Enabled {
		return false
	}
	if !CanCompact(messages) {
		return false
	}
	tokens := estimateContextTokens(messages)
	return needsCompaction(tokens, settings)
}
