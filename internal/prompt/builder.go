package prompt

import (
	"os"
	"runtime"
	"strings"
	"time"

	"crobot/internal/config"
)

// Build assembles the system prompt from the user config, replacing placeholders
// and appending dynamic context about the current environment.
func Build(cfg config.AgentConfig, cwd string) string {
	prompt := cfg.SystemPrompt
	if prompt == "" {
		prompt = config.DEFAULTS.SystemPrompt
	}

	if cfg.AppendPrompt != "" {
		prompt += "\n\n" + cfg.AppendPrompt
	}

	// Replace {cwd} with actual working directory.
	prompt = strings.ReplaceAll(prompt, "{cwd}", cwd)

	// Append dynamic context.
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n")
	b.WriteString("Current date: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04 MST"))
	b.WriteString("\n")
	b.WriteString("Shell: ")
	if bash := os.Getenv("SHELL"); bash != "" {
		b.WriteString(bash)
	} else {
		b.WriteString("unknown")
	}
	b.WriteString("\n")
	b.WriteString("Platform: ")
	b.WriteString(runtime.GOOS)
	b.WriteString("/")
	b.WriteString(runtime.GOARCH)
	b.WriteString("\n")

	return b.String()
}
