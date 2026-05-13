// Package mcpconfig loads MCP server configuration from ~/.crobot/mcp.json.
package mcpconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SettingsConfig holds global MCP settings.
type SettingsConfig struct {
	IdleTimeout      int  `json:"idleTimeout"`
	DirectTools      any  `json:"directTools"`
	DisableProxyTool bool `json:"disableProxyTool"`
}

// ServerConfig defines a single MCP server.
type ServerConfig struct {
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	CWD          string            `json:"cwd,omitempty"`
	URL          string            `json:"url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Auth         string            `json:"auth,omitempty"`
	Lifecycle    string            `json:"lifecycle,omitempty"`
	IdleTimeout  int               `json:"idleTimeout,omitempty"`
	DirectTools  any               `json:"directTools,omitempty"`
	ExcludeTools []string          `json:"excludeTools,omitempty"`
	Debug        bool              `json:"debug,omitempty"`
}

// Config is the top-level MCP configuration.
type Config struct {
	Settings   SettingsConfig          `json:"settings"`
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// DirectToolsMode describes whether and which tools are directly exposed.
type DirectToolsMode struct {
	All   bool
	Names []string
}

// ParseDirectTools converts the DirectTools field into a structured mode.
func ParseDirectTools(v any) DirectToolsMode {
	if v == nil {
		return DirectToolsMode{}
	}
	switch val := v.(type) {
	case bool:
		return DirectToolsMode{All: val}
	case []any:
		var names []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
		return DirectToolsMode{Names: names}
	default:
		return DirectToolsMode{}
	}
}

// ConfigPath returns the path to the MCP configuration file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %w", err)
	}
	return filepath.Join(home, ".crobot", "mcp.json"), nil
}

// CachePath returns the path to the MCP metadata cache file.
func CachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %w", err)
	}
	return filepath.Join(home, ".crobot", "mcp-cache.json"), nil
}

// LoadIfExists reads and validates the MCP config if it exists.
// Returns (nil, false, nil) if the file is absent.
func LoadIfExists() (*Config, bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s: %w", path, err)
	}

	var raw Config
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false, fmt.Errorf("invalid %s: %w", path, err)
	}

	// Validate and expand.
	cfg, err := normalize(&raw)
	if err != nil {
		return nil, false, err
	}

	if len(cfg.MCPServers) == 0 {
		return nil, false, nil
	}

	return cfg, true, nil
}

// normalize validates and expands config values.
func normalize(raw *Config) (*Config, error) {
	cfg := *raw

	if cfg.Settings.IdleTimeout < 0 {
		return nil, fmt.Errorf("invalid settings.idleTimeout: %d (must be >= 0)", cfg.Settings.IdleTimeout)
	}
	if cfg.Settings.IdleTimeout == 0 {
		cfg.Settings.IdleTimeout = 10
	}

	cfg.MCPServers = make(map[string]ServerConfig, len(raw.MCPServers))
	for name, srv := range raw.MCPServers {
		normalized, err := normalizeServer(name, srv, cfg.Settings.IdleTimeout)
		if err != nil {
			return nil, err
		}
		cfg.MCPServers[name] = normalized
	}

	return &cfg, nil
}

func normalizeServer(name string, raw ServerConfig, globalIdleTimeout int) (ServerConfig, error) {
	if raw.Lifecycle == "" {
		raw.Lifecycle = "lazy"
	}
	if raw.Lifecycle != "lazy" && raw.Lifecycle != "eager" && raw.Lifecycle != "keep-alive" {
		return ServerConfig{}, fmt.Errorf("server %q: invalid lifecycle %q (valid: lazy, eager, keep-alive)", name, raw.Lifecycle)
	}

	if raw.Command == "" && raw.URL == "" {
		return ServerConfig{}, fmt.Errorf("server %q: command or url is required", name)
	}

	if raw.IdleTimeout == 0 {
		raw.IdleTimeout = globalIdleTimeout
	}
	if raw.IdleTimeout < 0 {
		return ServerConfig{}, fmt.Errorf("server %q: invalid idleTimeout %d", name, raw.IdleTimeout)
	}

	// Expand env vars in env values and args.
	raw.Env = expandMap(raw.Env)
	for i, arg := range raw.Args {
		raw.Args[i] = expandEnv(arg)
	}

	// Expand ~ in cwd.
	if strings.HasPrefix(raw.CWD, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ServerConfig{}, fmt.Errorf("server %q: cannot expand cwd: %w", name, err)
		}
		raw.CWD = filepath.Join(home, raw.CWD[2:])
	}

	return raw, nil
}

var envVarRe = regexp.MustCompile(`\$\{(\w+)\}|\$env:(\w+)`)

func expandEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		if strings.HasPrefix(match, "${") {
			key := match[2 : len(match)-1]
			return os.Getenv(key)
		}
		// $env:KEY
		key := match[5:]
		return os.Getenv(key)
	})
}

func expandMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = expandEnv(v)
	}
	return out
}
