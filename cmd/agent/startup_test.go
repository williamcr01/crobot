package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crobot/internal/config"
)

func TestCreateStartupProvider_NoAuthDoesNotRewriteConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAgentConfig(t, home, `{"provider":"anthropic","model":"claude-sonnet-4-5","thinking":"high"}`)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	prov, warning, err := createStartupProvider(cfg, config.AuthConfig{})
	if err != nil {
		t.Fatalf("createStartupProvider: %v", err)
	}
	if prov != nil {
		t.Fatal("expected no provider")
	}
	if warning == "" {
		t.Fatal("expected missing-provider warning")
	}
	if cfg.Provider != "" || cfg.Model != "" {
		t.Fatalf("expected in-memory provider/model disabled, got provider=%q model=%q", cfg.Provider, cfg.Model)
	}

	content := readAgentConfig(t, home)
	for _, want := range []string{`"provider":"anthropic"`, `"model":"claude-sonnet-4-5"`, `"thinking":"high"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected persisted %s to be preserved, got %s", want, content)
		}
	}
}

func TestCreateStartupProvider_ConfiguredProviderWithoutCredentialsDoesNotRewriteConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAgentConfig(t, home, `{"provider":"anthropic","model":"claude-sonnet-4-5","thinking":"high"}`)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	auth := config.AuthConfig{"openrouter": {Type: "apiKey", APIKey: "sk-or-test"}}

	prov, warning, err := createStartupProvider(cfg, auth)
	if err != nil {
		t.Fatalf("createStartupProvider: %v", err)
	}
	if prov != nil {
		t.Fatal("expected no provider")
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if cfg.Provider != "" || cfg.Model != "" {
		t.Fatalf("expected in-memory provider/model disabled, got provider=%q model=%q", cfg.Provider, cfg.Model)
	}

	content := readAgentConfig(t, home)
	for _, want := range []string{`"provider":"anthropic"`, `"model":"claude-sonnet-4-5"`, `"thinking":"high"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected persisted %s to be preserved, got %s", want, content)
		}
	}
}

func writeAgentConfig(t *testing.T, home, content string) {
	t.Helper()
	path := filepath.Join(home, ".crobot", "agent.config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAgentConfig(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".crobot", "agent.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
