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
	"crobot/internal/conversation"
	"crobot/internal/provider"
)

// MessageItem is the compaction-facing conversation message.
type MessageItem struct {
	Role      string // "user", "assistant", "system", "error", "compaction"
	Content   string
	Reasoning string
	ToolCalls []ToolRenderItem
}

// ToolRenderItem holds one tool call/result for compaction and display.
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
// The total output is capped at maxSerializedChars to prevent the summarization
// prompt from exceeding model context limits and causing hangs.
func serializeMessages(messages []MessageItem) string {
	const maxSerializedChars = 100_000
	var b strings.Builder
	truncated := false
	for _, msg := range messages {
		if b.Len() >= maxSerializedChars {
			truncated = true
			break
		}
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
	if truncated {
		b.WriteString("\n[ ... earlier messages truncated to stay within context limits ... ]\n")
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

// buildMessagesForAgent converts messages to provider.Messages for the agent runner.
func buildMessagesForAgent(messages []MessageItem) []provider.Message {
	return conversation.MessagesToProvider(messagesToConversation(messages))
}

func messagesToConversation(messages []MessageItem) []conversation.Message {
	result := make([]conversation.Message, 0, len(messages))
	for _, msg := range messages {
		convMsg := conversation.Message{
			Role:      msg.Role,
			Content:   msg.Content,
			Reasoning: msg.Reasoning,
			ToolCalls: make([]conversation.ToolResult, len(msg.ToolCalls)),
		}
		for i, tc := range msg.ToolCalls {
			convMsg.ToolCalls[i] = conversation.ToolResult{
				Name:    tc.Name,
				CallID:  tc.CallID,
				Args:    tc.RawArgs,
				ArgsStr: tc.Args,
				Output:  tc.Output,
			}
		}
		result = append(result, convMsg)
	}
	return result
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

// canModelHandleSummary checks whether a model's context window is large enough
// to handle the summarization input. It estimates input tokens from serialized
// messages plus prompt overhead and compares against the model's context window,
// leaving 20% headroom for output tokens.
func canModelHandleSummary(provName, modelID string, messages []MessageItem) error {
	if modelID == "" {
		return fmt.Errorf("no model specified")
	}

	conversationText := serializeMessages(messages)
	// Estimate: serialized chars/4 + system prompt + user prompt + overhead (~500 tokens)
	estimatedInputTokens := len(conversationText)/4 + 500
	contextWindow := provider.ContextWindowForModel(provName, modelID)

	// Leave 20% headroom for output tokens and tokenizer variance.
	if estimatedInputTokens > contextWindow*80/100 {
		return fmt.Errorf("estimated summarization input ~%d tokens exceeds %d token context window (model %q has %d tokens)",
			estimatedInputTokens, contextWindow*80/100, modelID, contextWindow)
	}
	return nil
}

// generateSummary calls the LLM to produce a compaction summary from messages.
// If previousSummary is non-empty, it uses the update prompt to merge new
// messages into the existing summary (iterative summarization).
func generateSummary(
	ctx context.Context,
	prov provider.Provider,
	modelID string,
	messages []MessageItem,
	customInstructions string,
	previousSummary string,
) (string, error) {
	conversationText := serializeMessages(messages)

	// Choose initial or update prompt based on whether we have a previous summary.
	var prompt string
	if previousSummary != "" {
		prompt = updateSummarizationPrompt
	} else {
		prompt = summarizationPrompt
	}
	if customInstructions != "" {
		prompt += "\n\nAdditional focus: " + customInstructions
	}

	var promptText string
	if previousSummary != "" {
		promptText = "<conversation>\n" + conversationText + "\n</conversation>\n\n" +
			"<previous-summary>\n" + previousSummary + "\n</previous-summary>\n\n" + prompt
	} else {
		promptText = "<conversation>\n" + conversationText + "\n</conversation>\n\n" + prompt
	}

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
	Summary      string
	TokensBefore int
	KeptMessages []MessageItem
	NewMessages  []MessageItem // replacement messages for the TUI
}

// Compact performs context compaction on the given messages.
//
// It estimates token usage, finds a cut point preserving keepRecentTokens,
// calls the LLM to summarize older messages, and returns the compacted
// message list.
//
// The provider is used for summarization. If compactionModel is non-empty,
// it overrides the model used for summarization.
//
// If previousSummary is non-empty, the LLM performs an iterative update
// instead of generating a fresh summary, preserving context from earlier
// compactions.
func Compact(
	ctx context.Context,
	prov provider.Provider,
	model string,
	settings config.CompactionConfig,
	messages []MessageItem,
	customInstructions string,
	previousSummary string,
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

	// Pre-flight: check if the summarization model can handle this context.
	if err := canModelHandleSummary(prov.Name(), summarizationModel, messagesToSummarize); err != nil {
		// If the user explicitly set a separate compaction model and it's too small,
		// fall back to the user's main model.
		if settings.Model != "" && settings.Model != model {
			if err2 := canModelHandleSummary(prov.Name(), model, messagesToSummarize); err2 != nil {
				return nil, fmt.Errorf("compaction model %q too small (%v); main model %q also too small (%v)",
					summarizationModel, err, model, err2)
			}
			summarizationModel = model
		} else {
			return nil, fmt.Errorf("model %q cannot handle summarization: %w", summarizationModel, err)
		}
	}

	summary, err := generateSummary(ctx, prov, summarizationModel, messagesToSummarize, customInstructions, previousSummary)
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
