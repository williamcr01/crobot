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
	if cfg.Provider != "anthropic" || cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected in-memory provider/model preserved, got provider=%q model=%q", cfg.Provider, cfg.Model)
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
	if cfg.Provider != "anthropic" || cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected in-memory provider/model preserved, got provider=%q model=%q", cfg.Provider, cfg.Model)
	}

	content := readAgentConfig(t, home)
	for _, want := range []string{`"provider":"anthropic"`, `"model":"claude-sonnet-4-5"`, `"thinking":"high"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected persisted %s to be preserved, got %s", want, content)
		}
	}
}

func TestParseStartupArgs(t *testing.T) {
	parsed, remaining, err := parseStartupArgs([]string{"--continue", "prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.continueRecent || parsed.noSession || parsed.sessionPath != "" || parsed.help {
		t.Fatalf("unexpected parsed args: %+v", parsed)
	}
	if len(remaining) != 1 || remaining[0] != "prompt" {
		t.Fatalf("unexpected remaining args: %v", remaining)
	}

	parsed, _, err = parseStartupArgs([]string{"--session", "/tmp/s.jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.sessionPath != "/tmp/s.jsonl" {
		t.Fatalf("expected session path, got %+v", parsed)
	}

	if _, _, err := parseStartupArgs([]string{"--no-session", "--continue"}); err == nil {
		t.Fatal("expected conflicting session args error")
	}
}

func TestParseStartupArgs_Version(t *testing.T) {
	for _, name := range []string{"--version", "-v"} {
		parsed, remaining, err := parseStartupArgs([]string{name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !parsed.showVersion {
			t.Fatalf("%s: expected showVersion=true", name)
		}
		if len(remaining) != 0 {
			t.Fatalf("%s: unexpected remaining: %v", name, remaining)
		}
	}
}

func TestParseStartupArgs_Help(t *testing.T) {
	for _, name := range []string{"--help"} {
		parsed, remaining, err := parseStartupArgs([]string{name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !parsed.help {
			t.Fatalf("%s: expected help=true", name)
		}
		if len(remaining) != 0 {
			t.Fatalf("%s: unexpected remaining: %v", name, remaining)
		}
	}
}

func TestParseStartupArgs_ShortHelp(t *testing.T) {
	for _, name := range []string{"-h"} {
		parsed, remaining, err := parseStartupArgs([]string{name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !parsed.help {
			t.Fatalf("%s: expected help=true", name)
		}
		if len(remaining) != 0 {
			t.Fatalf("%s: unexpected remaining: %v", name, remaining)
		}
	}
}

func TestParseStartupArgs_HelpSubcommand(t *testing.T) {
	for _, name := range []string{"help"} {
		parsed, remaining, err := parseStartupArgs([]string{name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !parsed.help {
			t.Fatalf("%s: expected help=true", name)
		}
		if len(remaining) != 0 {
			t.Fatalf("%s: unexpected remaining: %v", name, remaining)
		}
	}
}

func TestParseStartupArgs_Skill(t *testing.T) {
	parsed, remaining, err := parseStartupArgs([]string{"--skill", "/tmp/skill.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.skillPaths) != 1 || parsed.skillPaths[0] != "/tmp/skill.md" {
		t.Fatalf("expected skill path, got %+v", parsed.skillPaths)
	}
	if len(remaining) != 0 {
		t.Fatalf("unexpected remaining: %v", remaining)
	}
}

func TestParseStartupArgs_SkillRepeatable(t *testing.T) {
	parsed, remaining, err := parseStartupArgs([]string{"--skill", "a.md", "--skill", "b.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.skillPaths) != 2 || parsed.skillPaths[0] != "a.md" || parsed.skillPaths[1] != "b.md" {
		t.Fatalf("expected 2 skill paths, got %+v", parsed.skillPaths)
	}
	if len(remaining) != 0 {
		t.Fatalf("unexpected remaining: %v", remaining)
	}
}

func TestParseStartupArgs_SkillMissingArg(t *testing.T) {
	_, _, err := parseStartupArgs([]string{"--skill"})
	if err == nil {
		t.Fatal("expected error for --skill without path")
	}
}

func TestCliHelpText(t *testing.T) {
	text := cliHelpText()

	checks := []string{
		"Crobot",
		"Usage:",
		"--help",
		"-h",
		"--version",
		"-v",
		"--continue",
		"-c",
		"--session <path>",
		"--no-session",
		"--skill <path>",
		"/help",
		"slash commands",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Errorf("cliHelpText() missing: %q", check)
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
