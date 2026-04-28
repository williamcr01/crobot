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
	SessionDir:    "~/.crobot/sessions",
	ShowBanner:    true,
	SlashCommands: true,
	Display: DisplayConfig{
		ToolDisplay: "grouped",
		Reasoning:   true,
		InputStyle:  "block",
	},
	Plugins: PluginConfig{
		Enabled:     true,
		Directories: []string{"~/.crobot/plugins"},
		Permissions: []string{"file_read", "file_write", "bash", "tool_call", "send_message"},
	},
}

// LoadConfig loads configuration from defaults, ~/.crobot/agent.config.json, and environment variables.
func LoadConfig() (*AgentConfig, error) {
	cfg := DEFAULTS
	configPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	if err := EnsureBaseConfig(); err != nil {
		return nil, err
	}

	// Merge from ~/.crobot/agent.config.json.
	if data, err := os.ReadFile(configPath); err == nil {
		var file AgentConfig
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", configPath, err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", configPath, err)
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
		if hasKey(raw, "showBanner") {
			cfg.ShowBanner = file.ShowBanner
		}
		if hasKey(raw, "slashCommands") {
			cfg.SlashCommands = file.SlashCommands
		}
		// Display nested merge.
		if file.Display.ToolDisplay != "" {
			cfg.Display.ToolDisplay = file.Display.ToolDisplay
		}
		if file.Display.InputStyle != "" {
			cfg.Display.InputStyle = file.Display.InputStyle
		}
		if hasNestedKey(raw, "display", "reasoning") {
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
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required: set it in %s, .env, or environment", configPath)
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

	if cfg.SessionDir, err = expandHome(cfg.SessionDir); err != nil {
		return nil, err
	}
	for i, dir := range cfg.Plugins.Directories {
		cfg.Plugins.Directories[i], err = expandHome(dir)
		if err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

// SaveConfig writes only runtime-selected values to ~/.crobot/agent.config.json.
// It preserves existing user-authored config and only updates provider, model,
// and thinking so defaults are not expanded into the file.
func SaveConfig(cfg *AgentConfig) error {
	configPath, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := EnsureBaseConfig(); err != nil {
		return err
	}
	raw := map[string]any{}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("invalid %s: %w", configPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", configPath, err)
	}

	raw["provider"] = cfg.Provider
	raw["model"] = cfg.Model
	raw["thinking"] = cfg.Thinking

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", configPath, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	return nil
}

func expandHome(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ConfigDir returns the Crobot user configuration directory.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %w", err)
	}
	return filepath.Join(home, ".crobot"), nil
}

// ConfigPath returns the Crobot user configuration file path.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agent.config.json"), nil
}

// EnsureBaseConfig creates ~/.crobot, ~/.crobot/plugins, and the base config file when missing.
func EnsureBaseConfig() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o755); err != nil {
		return fmt.Errorf("create plugin directory: %w", err)
	}
	path := filepath.Join(dir, "agent.config.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	data, err := json.MarshalIndent(map[string]any{}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal base config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func hasKey(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

func hasNestedKey(raw map[string]json.RawMessage, parent, key string) bool {
	parentRaw, ok := raw[parent]
	if !ok {
		return false
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(parentRaw, &nested); err != nil {
		return false
	}
	_, ok = nested[key]
	return ok
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
