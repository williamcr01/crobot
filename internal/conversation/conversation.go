// Package conversation defines the canonical message representation shared
// between the agent runner, TUI, and compaction.  It is the single source of
// truth for what was said and done in a conversation.
package conversation

import (
	"crobot/internal/provider"
)

// Role constants for message roles.
const (
	RoleUser        = "user"
	RoleAssistant   = "assistant"
	RoleSystem      = "system"
	RoleTool        = "tool"
	RoleCompaction  = "compaction"
)

// ToolResult bundles a tool call with its result in a single structure.
// This is richer than provider.Message (which separates tool calls from
// results into distinct messages) and is the canonical representation
// of a completed tool interaction.
type ToolResult struct {
	Name    string
	CallID  string
	Args    map[string]any
	ArgsStr string // pre-formatted display string for the TUI
	Output  string
}

// Message is the canonical conversation message.  It contains everything
// needed to reproduce the LLM-facing message list and to render the TUI.
// UI-only annotations (ephemeral flag, formatted usage strings) are
// intentionally excluded — those belong in the TUI layer.
type Message struct {
	Role      string       // RoleUser, RoleAssistant, RoleSystem, RoleTool, or RoleCompaction
	Content   string
	Reasoning string
	ToolCalls []ToolResult
	Usage     *provider.Usage
}

// MessagesToProvider converts canonical messages into the flat
// provider.Message list that the LLM API expects.  Assistant messages
// with tool calls are split into an assistant message (with ToolCalls)
// followed by individual tool-result messages.  Compaction messages are
// sent as system messages.
//
// This is the single conversion function — both the TUI and compaction
// must use it to build the LLM-facing message list.
func MessagesToProvider(msgs []Message) []provider.Message {
	var out []provider.Message
	for _, msg := range msgs {
		out = append(out, messageToProvider(msg)...)
	}
	return out
}

// ProviderToMessages converts a flat provider.Message list (LLM protocol
// format) back into bundled conversation.Message entries.  Assistant tool
// calls are paired with their subsequent tool-result messages.
func ProviderToMessages(msgs []provider.Message) []Message {
	var out []Message
	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]
		switch msg.Role {
		case RoleUser, RoleSystem:
			out = append(out, Message{Role: msg.Role, Content: msg.Content})
		case RoleAssistant:
			convMsg := Message{
				Role:      RoleAssistant,
				Content:   msg.Content,
				Reasoning: msg.ReasoningContent,
			}
			for _, tc := range msg.ToolCalls {
				tr := ToolResult{
					Name:   tc.Name,
					CallID: tc.ID,
					Args:   tc.Args,
				}
				// Look ahead for the matching tool-result message.
				for j := i + 1; j < len(msgs); j++ {
					if msgs[j].Role == RoleTool && msgs[j].ToolCallID == tc.ID {
						tr.Output = msgs[j].Content
						break
					}
				}
				convMsg.ToolCalls = append(convMsg.ToolCalls, tr)
			}
			out = append(out, convMsg)
		}
		// Tool messages are consumed inline above.
	}
	return out
}

func messageToProvider(msg Message) []provider.Message {
	switch msg.Role {
	case RoleUser, RoleSystem, RoleCompaction:
		role := msg.Role
		if role == RoleCompaction {
			role = RoleSystem
		}
		return []provider.Message{{Role: role, Content: msg.Content}}

	case RoleAssistant:
		if len(msg.ToolCalls) == 0 {
			return []provider.Message{{
				Role:             RoleAssistant,
				Content:          msg.Content,
				ReasoningContent: msg.Reasoning,
			}}
		}

		// Assistant message with tool calls: emit the assistant message
		// followed by individual tool-result messages.
		llmMsg := provider.Message{
			Role:             RoleAssistant,
			Content:          msg.Content,
			ReasoningContent: msg.Reasoning,
		}
		for _, tc := range msg.ToolCalls {
			if tc.CallID != "" {
				llmMsg.ToolCalls = append(llmMsg.ToolCalls, provider.ToolCall{
					Name: tc.Name,
					ID:   tc.CallID,
					Args: tc.Args,
				})
			}
		}
		out := []provider.Message{llmMsg}
		for _, tc := range msg.ToolCalls {
			if tc.Output != "" {
				out = append(out, provider.Message{
					Role:       RoleTool,
					ToolCallID: tc.CallID,
					Content:    tc.Output,
				})
			}
		}
		return out

	default:
		return []provider.Message{{Role: RoleUser, Content: msg.Content}}
	}
}
