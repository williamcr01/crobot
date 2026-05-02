package main

import (
	"context"
	"fmt"
	"os"

	"crobot/internal/agent"
	"crobot/internal/config"
	"crobot/internal/conversation"
	"crobot/internal/provider"
	"crobot/internal/runtime"
	"crobot/internal/skills"
	"crobot/internal/tools"
)

// runHeadless executes a single prompt and streams the response to stdout.
// It bypasses the TUI and uses the frontend-agnostic runtime directly.
func runHeadless(
	cfg *config.AgentConfig,
	prov provider.Provider,
	toolReg *tools.Registry,
	plugins agent.PluginManager,
	skillsList []skills.Skill,
	promptText string,
) {
	cwd, _ := os.Getwd()
	msgs := []conversation.Message{
		{Role: conversation.RoleUser, Content: promptText},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := runHeadlessAgent(ctx, cfg, prov, cwd, msgs, toolReg, plugins, skillsList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

// runHeadlessAgent runs the agent loop and writes text deltas to stdout.
func runHeadlessAgent(
	ctx context.Context,
	cfg *config.AgentConfig,
	prov provider.Provider,
	cwd string,
	messages []conversation.Message,
	toolReg *tools.Registry,
	plugins agent.PluginManager,
	skillsList []skills.Skill,
) error {
	_, err := runtime.RunAgent(ctx, runtime.AgentRequest{
		Config:   cfg,
		Provider: prov,
		ToolReg:  toolReg,
		Plugins:  plugins,
		Skills:   skillsList,
		CWD:      cwd,
		Messages: messages,
		OnEvent:  headlessEventHandler,
	})
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
