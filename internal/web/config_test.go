package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// Override config path lookup.
	origConfigPath := ConfigPath
	ConfigPath = func() (string, error) { return filepath.Join(dir, ".crobot", "web-search.json"), nil }
	defer func() { ConfigPath = origConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HasAnyProvider() {
		t.Error("expected no providers configured")
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	crobotDir := filepath.Join(dir, ".crobot")
	if err := os.MkdirAll(crobotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"exaApiKey": "exa-test-key", "perplexityApiKey": "pplx-test-key"}`
	if err := os.WriteFile(filepath.Join(crobotDir, "web-search.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	origConfigPath := ConfigPath
	ConfigPath = func() (string, error) { return filepath.Join(dir, ".crobot", "web-search.json"), nil }
	defer func() { ConfigPath = origConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExaAPIKey != "exa-test-key" {
		t.Errorf("expected exa-test-key, got %q", cfg.ExaAPIKey)
	}
	if cfg.PerplexityAPIKey != "pplx-test-key" {
		t.Errorf("expected pplx-test-key, got %q", cfg.PerplexityAPIKey)
	}
	if !cfg.HasAnyProvider() {
		t.Error("expected providers configured")
	}
}

func TestLoadConfig_EnvFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("EXA_API_KEY", "exa-env-key")
	t.Setenv("GEMINI_API_KEY", "gemini-env-key")

	origConfigPath := ConfigPath
	ConfigPath = func() (string, error) { return filepath.Join(dir, ".crobot", "web-search.json"), nil }
	defer func() { ConfigPath = origConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExaAPIKey != "exa-env-key" {
		t.Errorf("expected exa-env-key from env, got %q", cfg.ExaAPIKey)
	}
	if cfg.GeminiAPIKey != "gemini-env-key" {
		t.Errorf("expected gemini-env-key from env, got %q", cfg.GeminiAPIKey)
	}
	if !cfg.HasAnyProvider() {
		t.Error("expected providers configured from env")
	}
}

func TestLoadConfig_FileOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	crobotDir := filepath.Join(dir, ".crobot")
	if err := os.MkdirAll(crobotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"exaApiKey": "exa-file-key"}`
	if err := os.WriteFile(filepath.Join(crobotDir, "web-search.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)
	t.Setenv("EXA_API_KEY", "exa-env-key")

	origConfigPath := ConfigPath
	ConfigPath = func() (string, error) { return filepath.Join(dir, ".crobot", "web-search.json"), nil }
	defer func() { ConfigPath = origConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// File value takes precedence (env only fills in when file value is empty).
	if cfg.ExaAPIKey != "exa-file-key" {
		t.Errorf("expected file value exa-file-key, got %q", cfg.ExaAPIKey)
	}
}

func TestHasAnyProvider(t *testing.T) {
	cfg := &Config{}
	if cfg.HasAnyProvider() {
		t.Error("empty config should report no providers")
	}
	cfg.ExaAPIKey = "key"
	if !cfg.HasAnyProvider() {
		t.Error("config with exa key should report has provider")
	}
}

func TestProviderPriority(t *testing.T) {
	cfg := &Config{
		BraveAPIKey:  "brave-key",
		TavilyAPIKey: "tavily-key",
		SerperAPIKey: "serper-key",
	}
	providers := cfg.ProviderPriority()
	if len(providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(providers))
	}
	if providers[0] != "brave" || providers[1] != "tavily" || providers[2] != "serper" {
		t.Errorf("unexpected priority order: %v", providers)
	}
}

func TestResolveProvider(t *testing.T) {
	cfg := &Config{
		ExaAPIKey: "exa-key",
		BraveAPIKey: "brave-key",
	}

	tests := []struct {
		name      string
		requested string
		want      ResolvedProvider
		wantOK    bool
	}{
		{"auto selects exa", "auto", "exa", true},
		{"empty selects auto", "", "exa", true},
		{"specific exa", "exa", "exa", true},
		{"specific brave", "brave", "brave", true},
		{"unavailable perplexity falls to auto", "perplexity", "exa", true},
		{"unavailable becomes auto first available", "tavily", "exa", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cfg.ResolveProvider(tt.requested)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveProvider_NoProviders(t *testing.T) {
	cfg := &Config{}
	got, ok := cfg.ResolveProvider("auto")
	if ok {
		t.Error("expected no provider available")
	}
	if got != "" {
		t.Errorf("expected empty provider, got %q", got)
	}
}
