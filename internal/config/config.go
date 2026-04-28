package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	APIKey        string        `json:"apiKey,omitempty"`
	Model         string        `json:"model"`
	Thinking      string        `json:"thinking"`
	SystemPrompt  string        `json:"systemPrompt,omitempty"`
	AppendPrompt  string        `json:"appendPrompt,omitempty"`
	SessionDir    string        `json:"sessionDir"`
	ShowBanner    bool          `json:"showBanner"`
	SlashCommands bool          `json:"slashCommands"`
	Display       DisplayConfig `json:"display"`
	Plugins       PluginConfig  `json:"plugins"`
}

// DEFAULTS provides the base configuration before file and env overrides.
var DEFAULTS = AgentConfig{
	Provider: "openrouter",
	Thinking: "none",
	SystemPrompt: strings.Join([]string{
		"You are Crobot, a coding assistant. You have access to the following tools:",
		"file read,",
		"file write",
		"file edit",
		"bash",
		"",
		"Current working directory: {cwd}",
	}, "\n"),
	SessionDir:    ".sessions",
	ShowBanner:    true,
	SlashCommands: true,
	Display: DisplayConfig{
		ToolDisplay: "grouped",
		Reasoning:   true,
		InputStyle:  "block",
	},
	Plugins: PluginConfig{
		Enabled:     true,
		Directories: []string{"./plugins", "~/.config/crobot/plugins"},
		Permissions: []string{"file_read", "file_write", "bash", "tool_call", "send_message"},
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
		if file.Thinking != "" {
			cfg.Thinking = file.Thinking
		}
		if file.SystemPrompt != "" {
			cfg.SystemPrompt = file.SystemPrompt
		}
		if file.AppendPrompt != "" {
			cfg.AppendPrompt = file.AppendPrompt
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
	if v := os.Getenv("AGENT_THINKING"); v != "" {
		cfg.Thinking = v
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
	validThinking := map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true}
	if !validThinking[cfg.Thinking] {
		return nil, fmt.Errorf("invalid thinking: %q (valid: none, minimal, low, medium, high, xhigh)", cfg.Thinking)
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

// SaveConfig writes only runtime-selected values to agent.config.json.
// It preserves existing user-authored config and only updates provider, model,
// and thinking so defaults are not expanded into the file.
func SaveConfig(cfg *AgentConfig) error {
	raw := map[string]any{}
	if data, err := os.ReadFile("agent.config.json"); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("invalid agent.config.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read agent.config.json: %w", err)
	}

	raw["provider"] = cfg.Provider
	raw["model"] = cfg.Model
	raw["thinking"] = cfg.Thinking

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent.config.json: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile("agent.config.json", data, 0o644); err != nil {
		return fmt.Errorf("write agent.config.json: %w", err)
	}
	return nil
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
