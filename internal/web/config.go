package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds web search configuration from ~/.crobot/web-search.json.
type Config struct {
	ExaAPIKey        string `json:"exaApiKey,omitempty"`
	PerplexityAPIKey string `json:"perplexityApiKey,omitempty"`
	GeminiAPIKey     string `json:"geminiApiKey,omitempty"`
	BraveAPIKey      string `json:"braveApiKey,omitempty"`
	TavilyAPIKey     string `json:"tavilyApiKey,omitempty"`
	SerperAPIKey     string `json:"serperApiKey,omitempty"`
	Provider         string `json:"provider,omitempty"` // auto, exa, perplexity, gemini, brave, tavily, serper
}

// ConfigPath returns the path to the web search config file.
// It is a variable to allow tests to override it.
var ConfigPath = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".crobot", "web-search.json"), nil
}

// LoadConfig reads the web search config file. Returns an empty config if the file doesn't exist.
// Falls back to reading individual API keys from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Environment variable fallbacks.
	applyEnvString("EXA_API_KEY", &cfg.ExaAPIKey)
	applyEnvString("PERPLEXITY_API_KEY", &cfg.PerplexityAPIKey)
	applyEnvString("GEMINI_API_KEY", &cfg.GeminiAPIKey)
	applyEnvString("BRAVE_API_KEY", &cfg.BraveAPIKey)
	applyEnvString("TAVILY_API_KEY", &cfg.TavilyAPIKey)
	applyEnvString("SERPER_API_KEY", &cfg.SerperAPIKey)

	return cfg, nil
}

// HasAnyProvider reports whether at least one search provider API key is configured.
func (c *Config) HasAnyProvider() bool {
	return c.ExaAPIKey != "" ||
		c.PerplexityAPIKey != "" ||
		c.GeminiAPIKey != "" ||
		c.BraveAPIKey != "" ||
		c.TavilyAPIKey != "" ||
		c.SerperAPIKey != ""
}

// ProviderPriority returns the ordered list of providers for auto selection.
// Providers without API keys are filtered out.
func (c *Config) ProviderPriority() []ResolvedProvider {
	var out []ResolvedProvider
	// Priority order: Exa > Perplexity > Gemini > Brave > Tavily > Serper
	entries := []struct {
		name string
		key  string
	}{
		{"exa", c.ExaAPIKey},
		{"perplexity", c.PerplexityAPIKey},
		{"gemini", c.GeminiAPIKey},
		{"brave", c.BraveAPIKey},
		{"tavily", c.TavilyAPIKey},
		{"serper", c.SerperAPIKey},
	}
	for _, e := range entries {
		if e.key != "" {
			out = append(out, ResolvedProvider(e.name))
		}
	}
	return out
}

// ResolveProvider resolves a requested provider string to the first available
// provider. Returns ("", false) if no providers are available.
func (c *Config) ResolveProvider(requested string) (ResolvedProvider, bool) {
	if requested == "" {
		requested = c.Provider
	}
	if requested == "" {
		requested = "auto"
	}

	if requested != "auto" {
		rp := ResolvedProvider(requested)
		if c.apiKey(rp) != "" {
			return rp, true
		}
		// Requested provider unavailable, fall through to auto.
	}

	available := c.ProviderPriority()
	if len(available) == 0 {
		return "", false
	}
	return available[0], true
}

func (c *Config) apiKey(p ResolvedProvider) string {
	switch p {
	case "exa":
		return c.ExaAPIKey
	case "perplexity":
		return c.PerplexityAPIKey
	case "gemini":
		return c.GeminiAPIKey
	case "brave":
		return c.BraveAPIKey
	case "tavily":
		return c.TavilyAPIKey
	case "serper":
		return c.SerperAPIKey
	}
	return ""
}

func applyEnvString(key string, dst *string) {
	if v := os.Getenv(key); v != "" && *dst == "" {
		*dst = v
	}
}
