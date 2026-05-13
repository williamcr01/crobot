package mcpmanager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"crobot/internal/mcpconfig"
	"crobot/internal/tools"
)

func TestNew_EmptyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{}
	mgr := New(cfg, filepath.Join(home, "cache.json"))

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestRegisterTools_ProxyTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {Command: "echo", Args: []string{"hello"}},
		},
	}
	mgr := New(cfg, filepath.Join(home, "cache.json"))
	defer mgr.Close()

	reg := tools.NewRegistry()
	mgr.RegisterTools(reg)

	if !reg.Has("mcp") {
		t.Fatal("expected mcp proxy tool to be registered")
	}

	tool, ok := reg.Get("mcp")
	if !ok {
		t.Fatal("expected to get mcp tool")
	}
	if tool.Source != "mcp" {
		t.Errorf("expected source mcp, got %s", tool.Source)
	}
	if tool.Name != "mcp" {
		t.Errorf("expected name mcp, got %s", tool.Name)
	}
}

func TestRegisterTools_ProxyDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{
		Settings: mcpconfig.SettingsConfig{
			DisableProxyTool: true,
		},
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {Command: "echo"},
		},
	}
	mgr := New(cfg, filepath.Join(home, "cache.json"))
	defer mgr.Close()

	reg := tools.NewRegistry()
	mgr.RegisterTools(reg)

	if reg.Has("mcp") {
		t.Fatal("expected mcp proxy tool to NOT be registered when disabled")
	}
}

func TestRegisterTools_DirectTools_NoCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {
				Command:     "echo",
				DirectTools: true,
			},
		},
	}
	mgr := New(cfg, filepath.Join(home, "cache.json"))
	defer mgr.Close()

	reg := tools.NewRegistry()
	mgr.RegisterTools(reg)

	// Proxy should still be registered.
	if !reg.Has("mcp") {
		t.Fatal("expected mcp proxy tool")
	}
	// Direct tool should not be registered because cache is empty.
	if reg.Has("mcp__test__something") {
		t.Fatal("expected no direct tools without cache")
	}
}

func TestRegisterTools_DirectTools_WithCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "tool_a", DisplayName: "test_tool_a", Description: "Tool A"},
		{Name: "tool_b", DisplayName: "test_tool_b", Description: "Tool B"},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {
				Command:     "echo",
				DirectTools: true,
			},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	reg := tools.NewRegistry()
	mgr.RegisterTools(reg)

	if !reg.Has("mcp__test__tool_a") {
		t.Fatal("expected direct tool mcp__test__tool_a")
	}
	if !reg.Has("mcp__test__tool_b") {
		t.Fatal("expected direct tool mcp__test__tool_b")
	}

	tool, _ := reg.Get("mcp__test__tool_a")
	if tool.Source != "mcp:test" {
		t.Errorf("expected source mcp:test, got %s", tool.Source)
	}
}

func TestRegisterTools_DirectTools_FilterByName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "tool_a", DisplayName: "test_tool_a", Description: "Tool A"},
		{Name: "tool_b", DisplayName: "test_tool_b", Description: "Tool B"},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {
				Command:     "echo",
				DirectTools: []any{"tool_a"},
			},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	reg := tools.NewRegistry()
	mgr.RegisterTools(reg)

	if !reg.Has("mcp__test__tool_a") {
		t.Fatal("expected direct tool mcp__test__tool_a")
	}
	if reg.Has("mcp__test__tool_b") {
		t.Fatal("expected tool_b to be excluded")
	}
}

func TestRegisterTools_ExcludeTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "tool_a", DisplayName: "test_tool_a", Description: "Tool A"},
		{Name: "tool_b", DisplayName: "test_tool_b", Description: "Tool B"},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {
				Command:      "echo",
				DirectTools:  true,
				ExcludeTools: []string{"tool_a"},
			},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	reg := tools.NewRegistry()
	mgr.RegisterTools(reg)

	if reg.Has("mcp__test__tool_a") {
		t.Fatal("expected tool_a to be excluded")
	}
	if !reg.Has("mcp__test__tool_b") {
		t.Fatal("expected tool_b to be registered")
	}
}

func TestStatus_NoServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{}
	mgr := New(cfg, filepath.Join(home, "cache.json"))
	defer mgr.Close()

	result, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if v, ok := m["enabled"].(bool); !ok || !v {
		t.Error("expected enabled true")
	}
	if v, ok := m["serverCount"].(int); !ok || v != 0 {
		t.Errorf("expected serverCount 0, got %v", m["serverCount"])
	}
}

func TestSearchTools_WithCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "screenshot", DisplayName: "test_screenshot", Description: "Take a screenshot"},
		{Name: "navigate", DisplayName: "test_navigate", Description: "Navigate to URL"},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {Command: "echo"},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	result, err := mgr.SearchTools(context.Background(), "screenshot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	results := m["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0].(map[string]any)
	if r["name"] != "test_screenshot" {
		t.Errorf("expected test_screenshot, got %v", r["name"])
	}
}

func TestDescribeTool_WithCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "screenshot", DisplayName: "test_screenshot", Description: "Take a screenshot", InputSchema: map[string]any{"type": "object"}},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {Command: "echo"},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	result, err := mgr.DescribeTool(context.Background(), "test_screenshot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["name"] != "test_screenshot" {
		t.Errorf("expected test_screenshot, got %v", m["name"])
	}
	if m["description"] != "Take a screenshot" {
		t.Errorf("expected description, got %v", m["description"])
	}
}

func TestDescribeTool_ServerNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {Command: "echo"},
		},
	}
	mgr := New(cfg, filepath.Join(home, "cache.json"))
	defer mgr.Close()

	_, err := mgr.DescribeTool(context.Background(), "unknown_tool")
	if err == nil {
		t.Fatal("expected error for unknown server")
	}
}

func TestDescribeTool_ToolNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "screenshot", DisplayName: "test_screenshot", Description: "Take a screenshot"},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {Command: "echo"},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	_, err := mgr.DescribeTool(context.Background(), "test_unknown")
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestParseDisplayName(t *testing.T) {
	server, tool, err := parseDisplayName("github_search_repositories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server != "github" {
		t.Errorf("expected server github, got %s", server)
	}
	if tool != "search_repositories" {
		t.Errorf("expected tool search_repositories, got %s", tool)
	}
}

func TestParseDisplayName_NoUnderscore(t *testing.T) {
	_, _, err := parseDisplayName("justaname")
	if err == nil {
		t.Fatal("expected error for name without underscore")
	}
}

func TestManager_Close(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &mcpconfig.Config{}
	mgr := New(cfg, filepath.Join(home, "cache.json"))

	err := mgr.Close()
	if err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestSearchTools_RespectExclude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cachePath := filepath.Join(home, "cache.json")
	writeTestCache(t, cachePath, []cachedTool{
		{Name: "tool_a", DisplayName: "test_tool_a", Description: "Tool A"},
		{Name: "tool_b", DisplayName: "test_tool_b", Description: "Tool B"},
	})

	cfg := &mcpconfig.Config{
		MCPServers: map[string]mcpconfig.ServerConfig{
			"test": {
				Command:      "echo",
				ExcludeTools: []string{"tool_a"},
			},
		},
	}
	mgr := New(cfg, cachePath)
	defer mgr.Close()

	result, err := mgr.SearchTools(context.Background(), "tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	results := m["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (tool_a excluded), got %d", len(results))
	}
	r := results[0].(map[string]any)
	if r["name"] != "test_tool_b" {
		t.Errorf("expected test_tool_b, got %v", r["name"])
	}
}

func writeTestCache(t *testing.T, path string, tools []cachedTool) {
	t.Helper()
	c := &cache{path: path}
	c.data.Servers = map[string]cacheServer{
		"test": {Tools: tools},
	}
	c.writeLocked()
	// Also load it so subsequent reads work.
	c.loaded = true
	os.WriteFile(path, func() []byte {
		data, _ := json.MarshalIndent(c.data, "", "  ")
		return append(data, '\n')
	}(), 0o644)
}
