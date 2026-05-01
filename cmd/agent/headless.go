package main

import (
	"context"
	"fmt"
	"os"

	"crobot/internal/agent"
	"crobot/internal/config"
	"crobot/internal/prompt"
	"crobot/internal/provider"
	"crobot/internal/skills"
	"crobot/internal/tools"
)

// runHeadless executes a single prompt and streams the response to stdout.
// It bypasses the TUI entirely, calling the agent runner directly.
func runHeadless(
	cfg *config.AgentConfig,
	prov provider.Provider,
	toolReg *tools.Registry,
	plugins agent.PluginManager,
	skillsList []skills.Skill,
	promptText string,
) {
	cwd, _ := os.Getwd()
	sysPrompt := prompt.Build(*cfg, cwd, skillsList)

	msgs := []provider.Message{
		{Role: "user", Content: promptText},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := runHeadlessAgent(ctx, prov, cfg.Model, cfg.Thinking, cfg.MaxTurns, sysPrompt, msgs, toolReg, plugins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

// runHeadlessAgent runs the agent loop and writes text deltas to stdout.
func runHeadlessAgent(
	ctx context.Context,
	prov provider.Provider,
	model string,
	thinking string,
	maxTurns int,
	systemPrompt string,
	messages []provider.Message,
	toolReg *tools.Registry,
	plugins agent.PluginManager,
) error {
	_, err := agent.RunWithThinking(
		ctx,
		prov,
		model,
		thinking,
		maxTurns,
		systemPrompt,
		messages,
		toolReg,
		plugins,
		headlessEventHandler,
	)
	return err
}

// headlessEventHandler streams text output to stdout as it arrives.
func headlessEventHandler(ev agent.Event) {
	switch ev.Type {
	case "text_delta":
		fmt.Print(ev.TextDelta)
		os.Stdout.Sync()
	case "error":
		if ev.Error != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", ev.Error)
		}
	}
}
