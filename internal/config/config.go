package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CompactionConfig controls automatic context compaction.
type CompactionConfig struct {
	Enabled          bool   `json:"enabled"`
	ReserveTokens    int    `json:"reserveTokens"`
	KeepRecentTokens int    `json:"keepRecentTokens"`
	Model            string `json:"model,omitempty"`
}

// PluginConfig controls WASM plugin loading and permissions.
type PluginConfig struct {
	Enabled     bool     `json:"enabled"`
	Directories []string `json:"directories"`
	Permissions []string `json:"permissions"`
}

// OpenRouterConfig controls OpenRouter-specific request behavior.
type OpenRouterConfig struct {
	Cache    bool `json:"cache"`
	CacheTTL int  `json:"cacheTTL,omitempty"`
}

// SessionsConfig controls session persistence and retention.
type SessionsConfig struct {
	RetentionDays        int  `json:"retentionDays"`
	MaxSessions          int  `json:"maxSessions"`
	KeepNamed            bool `json:"keepNamed"`
	PruneOnStartup       bool `json:"pruneOnStartup"`
	PruneEmptyAfterHours int  `json:"pruneEmptyAfterHours"`
}

// AgentConfig is the top-level configuration for the agent.
type AgentConfig struct {
	Provider      string           `json:"provider"`
	Model         string           `json:"model"`
	Thinking      string           `json:"thinking"`
	MaxTurns      int              `json:"maxTurns"`
	SystemPrompt  string           `json:"systemPrompt,omitempty"`
	AppendPrompt  string           `json:"appendPrompt,omitempty"`
	SessionDir    string           `json:"sessionDir"`
	ShowBanner    bool             `json:"showBanner"`
	SlashCommands bool             `json:"slashCommands"`
	Reasoning     bool             `json:"reasoning"`
	Alignment     string           `json:"alignment"`
	Theme         string           `json:"theme"`
	Compaction    CompactionConfig  `json:"compaction"`
	Sessions      SessionsConfig    `json:"sessions"`
	Plugins       PluginConfig      `json:"plugins"`
	OpenRouter    OpenRouterConfig  `json:"openrouter"`

	HasAuthorizedProvider bool `json:"-"`
}

// DEFAULTS provides the base configuration before file and env overrides.
var DEFAULTS = AgentConfig{
	Provider: "",
	Model:    "",
	Thinking: "none",
	MaxTurns: -1,
	SystemPrompt: strings.Join([]string{
		"You are Crobot, a coding assistant. You have access to the following tools:",
		"file read,",
		"file write,",
		"file edit,",
		"bash,",
		"",
		"Current working directory: {cwd}",
	}, "\n"),
	SessionDir:    "~/.crobot/sessions",
	ShowBanner:    true,
	SlashCommands: true,
	Reasoning:     true,
	Alignment:     "left",
	Compaction: CompactionConfig{
		Enabled:          true,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	},
	Sessions: SessionsConfig{
		RetentionDays:        30,
		MaxSessions:          50,
		KeepNamed:            true,
		PruneOnStartup:       true,
		PruneEmptyAfterHours: 24,
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
	if err := EnsureAuth(); err != nil {
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
		if file.Model != "" {
			cfg.Model = file.Model
		}
		if file.Thinking != "" {
			cfg.Thinking = file.Thinking
		}
		if hasKey(raw, "maxTurns") {
			cfg.MaxTurns = file.MaxTurns
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
		if file.Alignment != "" {
			cfg.Alignment = file.Alignment
		}
		if file.Theme != "" {
			cfg.Theme = file.Theme
		}
		if hasKey(raw, "reasoning") {
			cfg.Reasoning = file.Reasoning
		} else if hasNestedKey(raw, "display", "reasoning") {
			// Backward compatibility for configs written before reasoning moved top-level.
			var legacy struct {
				Display struct {
					Reasoning bool `json:"reasoning"`
				} `json:"display"`
			}
			if err := json.Unmarshal(data, &legacy); err != nil {
				return nil, fmt.Errorf("invalid %s: %w", configPath, err)
			}
			cfg.Reasoning = legacy.Display.Reasoning
		}
		// Compaction nested merge.
		if hasNestedKey(raw, "compaction", "enabled") {
			cfg.Compaction.Enabled = file.Compaction.Enabled
		}
		if hasNestedKey(raw, "compaction", "reserveTokens") {
			cfg.Compaction.ReserveTokens = file.Compaction.ReserveTokens
		}
		if hasNestedKey(raw, "compaction", "keepRecentTokens") {
			cfg.Compaction.KeepRecentTokens = file.Compaction.KeepRecentTokens
		}
		if file.Compaction.Model != "" {
			cfg.Compaction.Model = file.Compaction.Model
		}

		// Sessions nested merge.
		if hasNestedKey(raw, "sessions", "retentionDays") {
			cfg.Sessions.RetentionDays = file.Sessions.RetentionDays
		}
		if hasNestedKey(raw, "sessions", "maxSessions") {
			cfg.Sessions.MaxSessions = file.Sessions.MaxSessions
		}
		if hasNestedKey(raw, "sessions", "keepNamed") {
			cfg.Sessions.KeepNamed = file.Sessions.KeepNamed
		}
		if hasNestedKey(raw, "sessions", "pruneOnStartup") {
			cfg.Sessions.PruneOnStartup = file.Sessions.PruneOnStartup
		}
		if hasNestedKey(raw, "sessions", "pruneEmptyAfterHours") {
			cfg.Sessions.PruneEmptyAfterHours = file.Sessions.PruneEmptyAfterHours
		}

		// Plugins nested merge.
		if len(file.Plugins.Directories) > 0 {
			cfg.Plugins.Directories = file.Plugins.Directories
		}
		if len(file.Plugins.Permissions) > 0 {
			cfg.Plugins.Permissions = file.Plugins.Permissions
		}

		// OpenRouter nested merge.
		if hasNestedKey(raw, "openrouter", "cache") {
			cfg.OpenRouter.Cache = file.OpenRouter.Cache
		}
		if hasNestedKey(raw, "openrouter", "cacheTTL") {
			cfg.OpenRouter.CacheTTL = file.OpenRouter.CacheTTL
		}
	}

	// Load .env file if present.
	if err := loadDotEnv(); err != nil {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	// Override from environment variables.
	if v := os.Getenv("AGENT_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("AGENT_THINKING"); v != "" {
		cfg.Thinking = v
	}
	if v := os.Getenv("AGENT_MAX_TURNS"); v != "" {
		var maxTurns int
		if _, err := fmt.Sscanf(v, "%d", &maxTurns); err != nil {
			return nil, fmt.Errorf("invalid AGENT_MAX_TURNS: %q", v)
		}
		cfg.MaxTurns = maxTurns
	}

	// Validate provider.
	validProviders := map[string]bool{"": true, "openrouter": true, "openai": true, "openai-responses-ws": true, "openai-codex": true, "deepseek": true, "anthropic": true}
	if !validProviders[cfg.Provider] {
		return nil, fmt.Errorf("unsupported provider: %q (supported: openrouter, openai, openai-responses-ws, openai-codex, deepseek, anthropic)", cfg.Provider)
	}

	validThinking := map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true}
	if !validThinking[cfg.Thinking] {
		return nil, fmt.Errorf("invalid thinking: %q (valid: none, minimal, low, medium, high, xhigh)", cfg.Thinking)
	}
	validAlignment := map[string]bool{"left": true, "centered": true}
	if !validAlignment[cfg.Alignment] {
		return nil, fmt.Errorf("invalid alignment: %q (valid: left, centered)", cfg.Alignment)
	}
	if cfg.MaxTurns < -1 {
		return nil, fmt.Errorf("invalid maxTurns: %d (must be -1 or greater)", cfg.MaxTurns)
	}
	if cfg.Sessions.RetentionDays < 0 {
		return nil, fmt.Errorf("invalid sessions.retentionDays: %d (must be >= 0)", cfg.Sessions.RetentionDays)
	}
	if cfg.Sessions.MaxSessions < 0 {
		return nil, fmt.Errorf("invalid sessions.maxSessions: %d (must be >= 0)", cfg.Sessions.MaxSessions)
	}
	if cfg.Sessions.PruneEmptyAfterHours < 0 {
		return nil, fmt.Errorf("invalid sessions.pruneEmptyAfterHours: %d (must be >= 0)", cfg.Sessions.PruneEmptyAfterHours)
	}
	if cfg.OpenRouter.CacheTTL < 0 || cfg.OpenRouter.CacheTTL > 86400 {
		return nil, fmt.Errorf("invalid openrouter.cacheTTL: %d (must be 0 or 1..86400)", cfg.OpenRouter.CacheTTL)
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

// SaveConfig writes only non-empty runtime fields to ~/.crobot/agent.config.json.
// Empty fields are left unchanged, preserving the existing value on disk.
// Call ClearProviderModel() to explicitly remove provider and model.
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

	if cfg.Provider != "" {
		raw["provider"] = cfg.Provider
	}
	if cfg.Model != "" {
		raw["model"] = cfg.Model
	}
	if cfg.Thinking != "" {
		raw["thinking"] = cfg.Thinking
	}
	if cfg.Alignment != "" {
		raw["alignment"] = cfg.Alignment
	}
	if cfg.Theme != "" {
		raw["theme"] = cfg.Theme
	}

	return writeRawConfig(configPath, raw)
}

// ClearProviderModel removes the provider and model keys from the config file.
// Used after logout or when no authorized provider exists.
func ClearProviderModel() error {
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

	delete(raw, "provider")
	delete(raw, "model")

	return writeRawConfig(configPath, raw)
}

func writeRawConfig(path string, raw map[string]any) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
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
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		return fmt.Errorf("create skills directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		return fmt.Errorf("create themes directory: %w", err)
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
