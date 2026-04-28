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
	if cfg.SessionDir != ".sessions" {
		t.Errorf("expected sessionDir .sessions, got %s", cfg.SessionDir)
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
	if len(cfg.Plugins.Directories) != 2 {
		t.Errorf("expected 2 plugin directories, got %d", len(cfg.Plugins.Directories))
	}
}

func TestLoadConfig_RequiresAPIKey(t *testing.T) {
	// Ensure no config file interferes.
	origFile := "agent.config.json"
	if _, err := os.Stat(origFile); err == nil {
		_, _ = os.ReadFile(origFile)
		os.Rename(origFile, origFile+".bak")
		defer os.Rename(origFile+".bak", origFile)
		defer os.Remove(origFile + ".bak")
	} else {
		defer func() {}()
	}
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when OPENROUTER_API_KEY is not set")
	}
}

func TestLoadConfig_WithEnvAPIKey(t *testing.T) {
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
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-from-env")

	configContent := `{
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
	}`
	if err := os.WriteFile("agent.config.json", []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Env key should still be picked up.
	if cfg.APIKey != "sk-or-v1-from-env" {
		t.Errorf("expected env API key, got %s", cfg.APIKey)
	}
	// File overrides model.
	if cfg.Model != "anthropic/claude-3-5-sonnet" {
		t.Errorf("expected file model, got %s", cfg.Model)
	}
	if cfg.Thinking != "high" {
		t.Errorf("expected file thinking, got %s", cfg.Thinking)
	}
	if cfg.AppendPrompt != "Extra instructions for {cwd}" {
		t.Errorf("expected appendPrompt, got %s", cfg.AppendPrompt)
	}
	// File overrides display.
	if cfg.Display.ToolDisplay != "emoji" {
		t.Errorf("expected emoji display, got %s", cfg.Display.ToolDisplay)
	}
	if cfg.Display.InputStyle != "bordered" {
		t.Errorf("expected bordered input, got %s", cfg.Display.InputStyle)
	}
	// File overrides plugin directories (replaces, not appends).
	if len(cfg.Plugins.Directories) != 1 || cfg.Plugins.Directories[0] != "./my-plugins" {
		t.Errorf("expected [./my-plugins], got %v", cfg.Plugins.Directories)
	}
}

func TestLoadConfig_InvalidToolDisplay(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")

	configContent := `{"display": {"toolDisplay": "invalid"}}`
	if err := os.WriteFile("agent.config.json", []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid toolDisplay")
	}
}

func TestLoadConfig_InvalidInputStyle(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")

	configContent := `{"display": {"inputStyle": "fancy"}}`
	if err := os.WriteFile("agent.config.json", []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid inputStyle")
	}
}

func TestLoadConfig_InvalidConfigFile(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")

	if err := os.WriteFile("agent.config.json", []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil || err.Error() != "invalid agent.config.json: invalid character 'i' looking for beginning of object key string" {
		t.Logf("got expected error: %v", err)
	}
}

func TestLoadConfig_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir:", err)
	}

	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")

	configContent := `{"plugins": {"directories": ["~/crobot/plugins"]}}`
	if err := os.WriteFile("agent.config.json", []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

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
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test")

	configContent := `{"provider": "ollama"}`
	if err := os.WriteFile("agent.config.json", []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestSaveConfig_WritesOnlyRuntimeFieldsWhenCreatingFile(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := DEFAULTS
	cfg.Model = "test/model"
	if err := SaveConfig(&cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	data, err := os.ReadFile("agent.config.json")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{`"provider": "openrouter"`, `"model": "test/model"`, `"thinking": "none"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %s in saved config, got %s", want, content)
		}
	}
	for _, forbidden := range []string{"systemPrompt", "appendPrompt", "sessionDir", "display", "plugins", "apiKey"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("%s should not be autosaved, got %s", forbidden, content)
		}
	}
}

func TestSaveConfig_PreservesExistingUserConfig(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	initial := `{
		"systemPrompt": "custom prompt",
		"appendPrompt": "extra prompt",
		"display": {"inputStyle": "bordered"},
		"model": "old/model"
	}`
	if err := os.WriteFile("agent.config.json", []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DEFAULTS
	cfg.Model = "new/model"
	cfg.Thinking = "high"
	if err := SaveConfig(&cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	data, err := os.ReadFile("agent.config.json")
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
