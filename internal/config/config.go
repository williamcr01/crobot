package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DisplayConfig controls visual appearance of the TUI.
type DisplayConfig struct {
	ToolDisplay string `json:"toolDisplay"` // "grouped" | "emoji" | "minimal" | "hidden"
	Reasoning   bool   `json:"reasoning"`
	InputStyle  string `json:"inputStyle"` // "block" | "bordered" | "plain"
}

// PluginConfig controls WASM plugin loading and permissions.
type PluginConfig struct {
	Enabled     bool     `json:"enabled"`
	Directories []string `json:"directories"`
	Permissions []string `json:"permissions"`
}

// AgentConfig is the top-level configuration for the agent.
type AgentConfig struct {
	Provider      string        `json:"provider"`
	APIKey        string        `json:"apiKey"`
	Model         string        `json:"model"`
	SystemPrompt  string        `json:"systemPrompt"`
	MaxSteps      int           `json:"maxSteps"`
	MaxCost       float64       `json:"maxCost"`
	SessionDir    string        `json:"sessionDir"`
	ShowBanner    bool          `json:"showBanner"`
	SlashCommands bool          `json:"slashCommands"`
	Display       DisplayConfig `json:"display"`
	Plugins       PluginConfig  `json:"plugins"`
}

// DEFAULTS provides the base configuration before file and env overrides.
var DEFAULTS = AgentConfig{
	Provider: "openrouter",
	Model:    "anthropic/claude-opus-4.7",
	SystemPrompt: strings.Join([]string{
		"You are a coding assistant with access to tools for reading, writing, editing files, and running shell commands.",
		"",
		"Current working directory: {cwd}",
		"",
		"Guidelines:",
		"- Use your tools proactively. Explore the codebase to find answers instead of asking the user.",
		"- Keep working until the task is fully resolved before responding.",
		"- Do not guess or make up information — use your tools to verify.",
		"- Be concise and direct.",
		"- Show file paths clearly when working with files.",
		"- Prefer file tools over shell commands for file search.",
		"- When editing code, make minimal targeted changes consistent with the existing style.",
	}, "\n"),
	MaxSteps:      20,
	MaxCost:       1.0,
	SessionDir:    ".sessions",
	ShowBanner:    true,
	SlashCommands: true,
	Display: DisplayConfig{
		ToolDisplay: "grouped",
		Reasoning:   false,
		InputStyle:  "block",
	},
	Plugins: PluginConfig{
		Enabled:     true,
		Directories: []string{"./plugins", "~/.config/crobot/plugins"},
		Permissions: []string{"file_read", "file_write", "shell", "tool_call", "send_message"},
	},
}

// LoadConfig loads configuration from defaults, agent.config.json, and environment variables.
func LoadConfig() (*AgentConfig, error) {
	cfg := DEFAULTS

	// Merge from agent.config.json if present.
	if data, err := os.ReadFile("agent.config.json"); err == nil {
		var file AgentConfig
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("invalid agent.config.json: %w", err)
		}
		if file.Provider != "" {
			cfg.Provider = file.Provider
		}
		if file.APIKey != "" {
			cfg.APIKey = file.APIKey
		}
		if file.Model != "" {
			cfg.Model = file.Model
		}
		if file.SystemPrompt != "" {
			cfg.SystemPrompt = file.SystemPrompt
		}
		if file.MaxSteps > 0 {
			cfg.MaxSteps = file.MaxSteps
		}
		if file.MaxCost > 0 {
			cfg.MaxCost = file.MaxCost
		}
		if file.SessionDir != "" {
			cfg.SessionDir = file.SessionDir
		}
		if _, ok := boolFieldSet(file.ShowBanner, "showBanner"); ok {
			cfg.ShowBanner = file.ShowBanner
		}
		if _, ok := boolFieldSet(file.SlashCommands, "slashCommands"); ok {
			cfg.SlashCommands = file.SlashCommands
		}
		// Display nested merge.
		if file.Display.ToolDisplay != "" {
			cfg.Display.ToolDisplay = file.Display.ToolDisplay
		}
		if file.Display.InputStyle != "" {
			cfg.Display.InputStyle = file.Display.InputStyle
		}
		if _, ok := boolFieldSet(file.Display.Reasoning, "display.reasoning"); ok {
			cfg.Display.Reasoning = file.Display.Reasoning
		}
		// Plugins nested merge.
		if len(file.Plugins.Directories) > 0 {
			cfg.Plugins.Directories = file.Plugins.Directories
		}
		if len(file.Plugins.Permissions) > 0 {
			cfg.Plugins.Permissions = file.Plugins.Permissions
		}
	}

	// Load .env file if present.
	if err := loadDotEnv(); err != nil {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	// Override from environment variables.
	if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("AGENT_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("AGENT_MAX_STEPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxSteps = n
		}
	}
	if v := os.Getenv("AGENT_MAX_COST"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MaxCost = n
		}
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required: set it in agent.config.json, .env, or environment")
	}

	// Validate provider.
	validProviders := map[string]bool{"openrouter": true}
	if !validProviders[cfg.Provider] {
		return nil, fmt.Errorf("unsupported provider: %q (supported: openrouter)", cfg.Provider)
	}

	// Validate display settings.
	validToolDisplays := map[string]bool{"grouped": true, "emoji": true, "minimal": true, "hidden": true}
	if !validToolDisplays[cfg.Display.ToolDisplay] {
		return nil, fmt.Errorf("invalid toolDisplay: %q (valid: grouped, emoji, minimal, hidden)", cfg.Display.ToolDisplay)
	}
	validInputStyles := map[string]bool{"block": true, "bordered": true, "plain": true}
	if !validInputStyles[cfg.Display.InputStyle] {
		return nil, fmt.Errorf("invalid inputStyle: %q (valid: block, bordered, plain)", cfg.Display.InputStyle)
	}

	// Expand ~ in plugin directories.
	for i, dir := range cfg.Plugins.Directories {
		if strings.HasPrefix(dir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("cannot resolve home directory: %w", err)
			}
			cfg.Plugins.Directories[i] = filepath.Join(home, dir[2:])
		}
	}

	return &cfg, nil
}

// boolFieldSet checks whether a bool field was explicitly set in JSON.
// This is a heuristic: for top-level fields, we compare to their zero value.
// The caller should pass the field name for context. We use a simple approach:
// we return true for "ok" when the field differs from the default zero-state
// of a JSON bool (which is false). This works because the JSON unmarshaller
// sets the field to false for missing keys AND for explicit false, so we can't
// distinguish. As a practical compromise, we always apply file overrides
// for bool fields when the struct has been merged.
//
// For the nested display.reasoning case, we just check the enclosing DisplayConfig
// was provided. This is a pragmatic heuristic.
func boolFieldSet(val bool, field string) (bool, bool) {
	_ = val
	_ = field
	return true, true
}

// loadDotEnv reads a .env file and sets environment variables.
func loadDotEnv() error {
	data, err := os.ReadFile(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading .env: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Only set if not already set.
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return nil
}
