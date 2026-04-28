package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := DEFAULTS
	if cfg.Provider != "openrouter" {
		t.Errorf("expected provider openrouter, got %s", cfg.Provider)
	}
	if cfg.Model != "" {
		t.Errorf("expected no default model, got %s", cfg.Model)
	}
	if cfg.Thinking != "none" {
		t.Errorf("expected thinking none, got %s", cfg.Thinking)
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
	if cfg.Display.ToolDisplay != "grouped" {
		t.Errorf("expected toolDisplay grouped, got %s", cfg.Display.ToolDisplay)
	}
	if cfg.Display.InputStyle != "block" {
		t.Errorf("expected inputStyle block, got %s", cfg.Display.InputStyle)
	}
	if !cfg.Plugins.Enabled {
		t.Error("expected plugins enabled")
	}
	if len(cfg.Plugins.Directories) != 1 || cfg.Plugins.Directories[0] != "~/.crobot/plugins" {
		t.Errorf("expected ~/.crobot/plugins, got %v", cfg.Plugins.Directories)
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
	if !cfg.Display.Reasoning {
		t.Fatal("empty config should preserve default display.reasoning=true")
	}
	if cfg.SessionDir != filepath.Join(home, ".crobot", "sessions") {
		t.Fatalf("expected expanded session dir, got %s", cfg.SessionDir)
	}
	if len(cfg.Plugins.Directories) != 1 || cfg.Plugins.Directories[0] != filepath.Join(home, ".crobot", "plugins") {
		t.Fatalf("expected expanded plugin dir, got %v", cfg.Plugins.Directories)
	}
}

func TestLoadConfig_RequiresAPIKey(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when OPENROUTER_API_KEY is not set")
	}
}

func TestLoadConfig_WithEnvAPIKey(t *testing.T) {
	withTempHome(t)
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-testkey")
	defer os.Unsetenv("OPENROUTER_API_KEY")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-testkey" {
		t.Errorf("expected test key, got %s", cfg.APIKey)
	}
}

func TestLoadConfig_WithEnvOverrides(t *testing.T) {
	withTempHome(t)
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-testkey")
	os.Setenv("AGENT_MODEL", "openai/gpt-4")
	defer os.Unsetenv("OPENROUTER_API_KEY")
	defer os.Unsetenv("AGENT_MODEL")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "openai/gpt-4" {
		t.Errorf("expected model override, got %s", cfg.Model)
	}
}

func TestLoadConfig_WithConfigFile(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-from-env")

	writeUserConfig(t, `{
		"model": "anthropic/claude-3-5-sonnet",
		"thinking": "high",
		"appendPrompt": "Extra instructions for {cwd}",
		"display": {
			"toolDisplay": "emoji",
			"inputStyle": "bordered"
		},
		"plugins": {
			"directories": ["./my-plugins"]
		}
	}`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-from-env" {
		t.Errorf("expected env API key, got %s", cfg.APIKey)
	}
	if cfg.Model != "anthropic/claude-3-5-sonnet" {
		t.Errorf("expected file model, got %s", cfg.Model)
	}
	if cfg.Thinking != "high" {
		t.Errorf("expected file thinking, got %s", cfg.Thinking)
	}
	if cfg.AppendPrompt != "Extra instructions for {cwd}" {
		t.Errorf("expected appendPrompt, got %s", cfg.AppendPrompt)
	}
	if cfg.Display.ToolDisplay != "emoji" {
		t.Errorf("expected emoji display, got %s", cfg.Display.ToolDisplay)
	}
	if cfg.Display.InputStyle != "bordered" {
		t.Errorf("expected bordered input, got %s", cfg.Display.InputStyle)
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
	if cfg.Display.InputStyle != DEFAULTS.Display.InputStyle {
		t.Fatalf("expected default input style, got %q", cfg.Display.InputStyle)
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
	writeUserConfig(t, `{"showBanner": false, "slashCommands": false, "display": {"reasoning": false}}`)

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
	if cfg.Display.Reasoning {
		t.Fatal("expected display.reasoning false override")
	}
}

func TestLoadConfig_InvalidToolDisplay(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"display": {"toolDisplay": "invalid"}}`)

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid toolDisplay")
	}
}

func TestLoadConfig_InvalidInputStyle(t *testing.T) {
	withTempHome(t)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")
	writeUserConfig(t, `{"display": {"inputStyle": "fancy"}}`)

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid inputStyle")
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
	for _, want := range []string{`"provider": "openrouter"`, `"model": "test/model"`, `"thinking": "none"`} {
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
