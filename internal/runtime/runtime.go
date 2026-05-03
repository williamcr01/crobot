// Package runtime provides frontend-agnostic orchestration for agent runs.
package runtime

import (
	"context"
	"fmt"

	"crobot/internal/agent"
	"crobot/internal/config"
	"crobot/internal/conversation"
	"crobot/internal/prompt"
	"crobot/internal/provider"
	"crobot/internal/skills"
	"crobot/internal/tools"
)

// AgentRequest contains all backend dependencies needed to run one agent turn.
// Frontends provide canonical conversation messages and consume streamed events.
type AgentRequest struct {
	Config   *config.AgentConfig
	Provider provider.Provider
	ToolReg  *tools.Registry
	Plugins  agent.PluginManager
	Skills   []skills.Skill
	CWD      string
	Messages []conversation.Message
	OnEvent  func(agent.Event)
}

// RunAgent builds the system prompt and provider-facing history, then runs the
// agent loop. It is independent of any frontend implementation.
func RunAgent(ctx context.Context, req AgentRequest) (*agent.Result, error) {
	if req.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if req.Provider == nil {
		return nil, fmt.Errorf("provider is required")
	}
	if req.ToolReg == nil {
		req.ToolReg = tools.NewRegistry()
	}
	cwd := req.CWD
	if cwd == "" {
		cwd = "."
	}

	sysPrompt := prompt.Build(*req.Config, cwd, req.Skills)
	llmMsgs := conversation.MessagesToProvider(req.Messages)

	return agent.RunWithOptions(
		ctx,
		req.Provider,
		req.Config.Model,
		req.Config.Thinking,
		req.Config.MaxTurns,
		sysPrompt,
		llmMsgs,
		req.ToolReg,
		req.Plugins,
		req.OnEvent,
		agent.RunOptions{
			Cache:    req.Config.Provider == "openrouter" && req.Config.OpenRouter.Cache,
			CacheTTL: req.Config.OpenRouter.CacheTTL,
		},
	)
}
