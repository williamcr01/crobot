package mcpconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIfExists_NoFile(t *testing.T) {
	home := withTempHome(t)
	configPath := filepath.Join(home, ".crobot", "mcp.json")

	cfg, exists, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false when file is missing")
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
	_ = configPath
}

func TestLoadIfExists_EmptyConfig(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{}`)

	cfg, exists, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false for empty config (no servers)")
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestLoadIfExists_EmptyServers(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{"mcpServers": {}}`)

	cfg, exists, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false for empty servers")
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestLoadIfExists_InvalidJSON(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{invalid`)

	_, _, err := LoadIfExists()
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
}

func TestLoadIfExists_ValidStdioServer(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			}
		}
	}`)

	cfg, exists, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected config to exist")
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.MCPServers))
	}
	srv := cfg.MCPServers["filesystem"]
	if srv.Command != "npx" {
		t.Errorf("expected command npx, got %s", srv.Command)
	}
	if len(srv.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(srv.Args))
	}
	if srv.Lifecycle != "lazy" {
		t.Errorf("expected default lifecycle lazy, got %s", srv.Lifecycle)
	}
	if srv.IdleTimeout != 10 {
		t.Errorf("expected default idleTimeout 10, got %d", srv.IdleTimeout)
	}
}

func TestLoadIfExists_MultipleServers(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			},
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"]
			}
		}
	}`)

	cfg, exists, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected config to exist")
	}
	if len(cfg.MCPServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cfg.MCPServers))
	}
}

func TestLoadIfExists_InvalidLifecycle(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{
		"mcpServers": {
			"test": {
				"command": "echo",
				"lifecycle": "always-on"
			}
		}
	}`)

	_, _, err := LoadIfExists()
	if err == nil || !strings.Contains(err.Error(), "invalid lifecycle") {
		t.Fatalf("expected lifecycle error, got %v", err)
	}
}

func TestLoadIfExists_MissingCommand(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{
		"mcpServers": {
			"test": {
				"args": ["hello"]
			}
		}
	}`)

	_, _, err := LoadIfExists()
	if err == nil || !strings.Contains(err.Error(), "command or url is required") {
		t.Fatalf("expected missing command error, got %v", err)
	}
}

func TestLoadIfExists_EnvExpansion(t *testing.T) {
	withTempHome(t)
	os.Setenv("GITHUB_TOKEN", "gh_token_123")
	defer os.Unsetenv("GITHUB_TOKEN")
	writeMCPConfig(t, `{
		"mcpServers": {
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"],
				"env": {
					"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}"
				}
			}
		}
	}`)

	cfg, _, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	srv := cfg.MCPServers["github"]
	if srv.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "gh_token_123" {
		t.Errorf("expected env expansion, got %v", srv.Env)
	}
}

func TestLoadIfExists_ArgsEnvExpansion(t *testing.T) {
	withTempHome(t)
	os.Setenv("TOKEN", "my_token")
	defer os.Unsetenv("TOKEN")
	writeMCPConfig(t, `{
		"mcpServers": {
			"test": {
				"command": "cmd",
				"args": ["--token", "${TOKEN}"]
			}
		}
	}`)

	cfg, _, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	srv := cfg.MCPServers["test"]
	if srv.Args[1] != "my_token" {
		t.Errorf("expected arg expansion, got %s", srv.Args[1])
	}
}

func TestLoadIfExists_Settings(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{
		"settings": {
			"idleTimeout": 5,
			"disableProxyTool": true,
			"directTools": true
		},
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			}
		}
	}`)

	cfg, _, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.IdleTimeout != 5 {
		t.Errorf("expected idleTimeout 5, got %d", cfg.Settings.IdleTimeout)
	}
	if !cfg.Settings.DisableProxyTool {
		t.Error("expected disableProxyTool true")
	}
}

func TestLoadIfExists_ServerIdleTimeoutOverride(t *testing.T) {
	withTempHome(t)
	writeMCPConfig(t, `{
		"settings": {
			"idleTimeout": 5
		},
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
				"idleTimeout": 30
			}
		}
	}`)

	cfg, _, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	srv := cfg.MCPServers["filesystem"]
	if srv.IdleTimeout != 30 {
		t.Errorf("expected server idleTimeout 30, got %d", srv.IdleTimeout)
	}
}

func TestLoadIfExists_CWDExpansion(t *testing.T) {
	home := withTempHome(t)
	writeMCPConfig(t, `{
		"mcpServers": {
			"test": {
				"command": "cmd",
				"cwd": "~/projects"
			}
		}
	}`)

	cfg, _, err := LoadIfExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	srv := cfg.MCPServers["test"]
	expected := filepath.Join(home, "projects")
	if srv.CWD != expected {
		t.Errorf("expected cwd %s, got %s", expected, srv.CWD)
	}
}

func TestParseDirectTools_Bool(t *testing.T) {
	m := ParseDirectTools(true)
	if !m.All {
		t.Error("expected All=true")
	}
	if len(m.Names) != 0 {
		t.Error("expected no names")
	}

	m = ParseDirectTools(false)
	if m.All || len(m.Names) != 0 {
		t.Error("expected all false for false")
	}
}

func TestParseDirectTools_StringArray(t *testing.T) {
	m := ParseDirectTools([]any{"tool_a", "tool_b"})
	if m.All {
		t.Error("expected All=false")
	}
	if len(m.Names) != 2 || m.Names[0] != "tool_a" || m.Names[1] != "tool_b" {
		t.Errorf("expected [tool_a tool_b], got %v", m.Names)
	}
}

func TestParseDirectTools_Nil(t *testing.T) {
	m := ParseDirectTools(nil)
	if m.All || len(m.Names) != 0 {
		t.Error("expected empty mode for nil")
	}
}

func TestParseDirectTools_InvalidType(t *testing.T) {
	m := ParseDirectTools(42)
	if m.All || len(m.Names) != 0 {
		t.Error("expected empty mode for invalid type")
	}
}

func TestConfigPath(t *testing.T) {
	home := withTempHome(t)
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".crobot", "mcp.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestCachePath(t *testing.T) {
	home := withTempHome(t)
	path, err := CachePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".crobot", "mcp-cache.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeMCPConfig(t *testing.T, content string) {
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
