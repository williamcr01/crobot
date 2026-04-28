package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := DEFAULTS
	if cfg.Provider != "" {
		t.Errorf("expected no default provider, got %s", cfg.Provider)
	}
	if cfg.Model != "" {
		t.Errorf("expected no default model, got %s", cfg.Model)
	}
	if cfg.Thinking != "none" {
		t.Errorf("expected thinking none, got %s", cfg.Thinking)
	}
	if cfg.MaxTurns != 50 {
		t.Errorf("expected maxTurns 50, got %d", cfg.MaxTurns)
	}
	if cfg.SessionDir != "~/.crobot/sessions" {
		t.Errorf("expected sessionDir ~/.crobot/sessions, got %s", cfg.SessionDir)
	}
	if !cfg.ShowBanner {
		t.Error("expected showBanner true")
	}
	if !cfg.SlashCommands {
		t.Error("expected slashCommands true")
	}
	if !cfg.Reasoning {
		t.Error("expected reasoning true")
	}
	if !cfg.Plugins.Enabled {
		t.Error("expected plugins enabled")
	}
	if len(cfg.Plugins.Directories) != 1 || cfg.Plugins.Directories[0] != "~/.crobot/plugins" {
		t.Errorf("expected ~/.crobot/plugins, got %v", cfg.Plugins.Directories)
	}
	if !cfg.Compaction.Enabled {
		t.Error("expected compaction enabled by default")
	}
	if cfg.Compaction.ReserveTokens != 16384 {
		t.Errorf("expected compaction reserveTokens 16384, got %d", cfg.Compaction.ReserveTokens)
	}
	if cfg.Compaction.KeepRecentTokens != 20000 {
		t.Errorf("expected compaction keepRecentTokens 20000, got %d", cfg.Compaction.KeepRecentTokens)
	}
}

func TestLoadConfig_BootstrapsUserConfigAndPlugins(t *testing.T) {
	home := withTempHome(t)
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-testkey")
	defer os.Unsetenv("OPENROUTER_API_KEY")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	configPath := filepath.Join(home, ".crobot", "agent.config.json")
	assertExists(t, configPath)
	assertExists(t, filepath.Join(home, ".crobot", "plugins"))
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "{}" {
		t.Fatalf("expected bootstrapped config to be empty object, got %s", string(data))
	}
	if cfg.Provider != DEFAULTS.Provider || cfg.Thinking != DEFAULTS.Thinking {
		t.Fatalf("expected defaults from empty config, got provider=%s thinking=%s", cfg.Provider, cfg.Thinking)
	}
	if !cfg.ShowBanner {
		t.Fatal("empty config should preserve default showBanner=true")
	}
	if !cfg.SlashCommands {
		t.Fatal("empty config should preserve default slashCommands=true")
	}
	if !cfg.Reasoning {
		t.Fatal("empty config should preserve default reasoning=true")
	}
	if cfg.SessionDir != filepath.Join(home, ".crobot", "sessions") {
		t.Fatalf("expected expanded session dir, got %s", cfg.SessionDir)
	}
	if len(cfg.Plugins.Directories) != 1 || cfg.Plugins.Directories[0] != filepath.Join(home, ".crobot", "plugins") {
		t.Fatalf("expected expanded plugin dir, got %v", cfg.Plugins.Directories)
	}
}

func TestLoadConfig_CreatesEmptyAuthFile(t *testing.T) {
	home := withTempHome(t)

	_, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".crobot", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "{}" {
		t.Fatalf("expected empty auth file, got %s", string(data))
	}
}

func TestLoadConfig_WithEnvOverrides(t *testing.T) {
	withTempHome(t)
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-testkey")
	os.Setenv("AGENT_MODEL", "openai/gpt-4")
	os.Setenv("AGENT_MAX_TURNS", "-1")
	defer os.Unsetenv("OPENROUTER_API_KEY")
	defer os.Unsetenv("AGENT_MODEL")
	defer os.Unsetenv("AGENT_MAX_TURNS")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "openai/gpt-4" {
		t.Errorf("expected model override, got %s", cfg.Model)
	}
	if cfg.MaxTurns != -1 {
		t.Errorf("expected maxTurns env override -1, got %d", cfg.MaxTurns)
	}
}

func TestLoadConfig_WithConfigFile(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-from-env")

	writeUserConfig(t, `{
		"model": "anthropic/claude-3-5-sonnet",
		"thinking": "high",
		"maxTurns": 12,
		"appendPrompt": "Extra instructions for {cwd}",
		"reasoning": false,
		"plugins": {
			"directories": ["./my-plugins"]
		}
	}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "anthropic/claude-3-5-sonnet" {
		t.Errorf("expected file model, got %s", cfg.Model)
	}
	if cfg.Thinking != "high" {
		t.Errorf("expected file thinking, got %s", cfg.Thinking)
	}
	if cfg.MaxTurns != 12 {
		t.Errorf("expected file maxTurns 12, got %d", cfg.MaxTurns)
	}
	if cfg.AppendPrompt != "Extra instructions for {cwd}" {
		t.Errorf("expected appendPrompt, got %s", cfg.AppendPrompt)
	}
	if cfg.Reasoning {
		t.Error("expected reasoning false")
	}
	if len(cfg.Plugins.Directories) != 1 || cfg.Plugins.Directories[0] != "./my-plugins" {
		t.Errorf("expected [./my-plugins], got %v", cfg.Plugins.Directories)
	}
}

func TestLoadConfig_PartialConfigUsesDefaults(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"appendPrompt": "Hello"}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppendPrompt != "Hello" {
		t.Fatalf("expected appendPrompt override, got %q", cfg.AppendPrompt)
	}
	if cfg.Provider != DEFAULTS.Provider {
		t.Fatalf("expected default provider, got %q", cfg.Provider)
	}
	if cfg.Thinking != DEFAULTS.Thinking {
		t.Fatalf("expected default thinking, got %q", cfg.Thinking)
	}
	if cfg.Reasoning != DEFAULTS.Reasoning {
		t.Fatalf("expected default reasoning, got %v", cfg.Reasoning)
	}
}

func TestLoadConfig_IgnoresProjectLocalConfig(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	if err := os.WriteFile("agent.config.json", []byte(`{"model":"project/model"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model == "project/model" {
		t.Fatal("project-local agent.config.json should be ignored")
	}
}

func TestLoadConfig_CanOverrideBoolDefaultsToFalse(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"showBanner": false, "slashCommands": false, "reasoning": false}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShowBanner {
		t.Fatal("expected showBanner false override")
	}
	if cfg.SlashCommands {
		t.Fatal("expected slashCommands false override")
	}
	if cfg.Reasoning {
		t.Fatal("expected reasoning false override")
	}
}

func TestLoadConfig_LegacyDisplayReasoning(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"display": {"reasoning": false}}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Reasoning {
		t.Fatal("expected legacy display.reasoning false override")
	}
}

func TestLoadConfig_InvalidMaxTurns(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"maxTurns": -2}`)

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid maxTurns") {
		t.Fatalf("expected invalid maxTurns error, got %v", err)
	}
}

func TestLoadConfig_InvalidConfigFile(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{invalid json`)

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestLoadConfig_TildeExpansion(t *testing.T) {
	home := withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"plugins": {"directories": ["~/crobot/plugins"]}}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, "crobot/plugins")
	if cfg.Plugins.Directories[0] != expected {
		t.Errorf("expected %s, got %s", expected, cfg.Plugins.Directories[0])
	}
}

func TestLoadConfig_UnsupportedProvider(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"provider": "ollama"}`)

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestSaveConfig_WritesOnlyRuntimeFieldsWhenCreatingFile(t *testing.T) {
	home := withTempHome(t)

	cfg := DEFAULTS
	cfg.Model = "test/model"
	if err := SaveConfig(&cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".crobot", "agent.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{`"provider": ""`, `"model": "test/model"`, `"thinking": "none"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %s in saved config, got %s", want, content)
		}
	}
}

func TestSaveConfig_PreservesExistingUserConfig(t *testing.T) {
	home := withTempHome(t)
	writeUserConfig(t, `{
		"systemPrompt": "custom prompt",
		"appendPrompt": "extra prompt",
		"display": {"inputStyle": "bordered"},
		"model": "old/model"
	}`)

	cfg := DEFAULTS
	cfg.Model = "new/model"
	cfg.Thinking = "high"
	if err := SaveConfig(&cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".crobot", "agent.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{`"systemPrompt": "custom prompt"`, `"appendPrompt": "extra prompt"`, `"inputStyle": "bordered"`, `"model": "new/model"`, `"thinking": "high"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %s in saved config, got %s", want, content)
		}
	}
}

func TestLoadConfig_CompactionSettings(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{
		"compaction": {
			"enabled": false,
			"reserveTokens": 8192,
			"keepRecentTokens": 10000,
			"model": "openai/gpt-4o-mini"
		}
	}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Compaction.Enabled {
		t.Error("expected compaction disabled")
	}
	if cfg.Compaction.ReserveTokens != 8192 {
		t.Errorf("expected reserveTokens 8192, got %d", cfg.Compaction.ReserveTokens)
	}
	if cfg.Compaction.KeepRecentTokens != 10000 {
		t.Errorf("expected keepRecentTokens 10000, got %d", cfg.Compaction.KeepRecentTokens)
	}
	if cfg.Compaction.Model != "openai/gpt-4o-mini" {
		t.Errorf("expected compaction model openai/gpt-4o-mini, got %s", cfg.Compaction.Model)
	}
}

func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeUserConfig(t *testing.T, content string) {
	t.Helper()
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}
