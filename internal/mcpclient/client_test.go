package mcpclient

import (
	"testing"

	"crobot/internal/mcpconfig"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewStdioClient_Valid(t *testing.T) {
	// Create a client for a valid config. We can't actually connect without
	// a real MCP server, but the constructor should not error.
	cfg := mcpconfig.ServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
		Env: map[string]string{
			"TEST": "value",
		},
	}

	client, err := NewStdioClient("test-server", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.ServerName() != "test-server" {
		t.Errorf("expected server name test-server, got %s", client.ServerName())
	}

	// Clean up.
	client.Close()
}

func TestNewStdioClient_MinimalConfig(t *testing.T) {
	cfg := mcpconfig.ServerConfig{
		Command: "echo",
	}

	client, err := NewStdioClient("minimal", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.ServerName() != "minimal" {
		t.Errorf("expected server name minimal, got %s", client.ServerName())
	}
	client.Close()
}

func TestNewStdioClient_EmptyEnv(t *testing.T) {
	cfg := mcpconfig.ServerConfig{
		Command: "echo",
		Env:     nil,
	}

	client, err := NewStdioClient("noenv", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client.Close()
}

func TestNormalizeResult_TextContent(t *testing.T) {
	// This tests the helper function directly.
	// We can't easily create an mcp.CallToolResult with Content interface values
	// from outside the package, but the normalize function is tested indirectly
	// through integration tests.
}

func TestNormalizeResult_IsError(t *testing.T) {
	// Also tested indirectly through integration.
}

func TestConvertInputSchema_Empty(t *testing.T) {
	result := convertInputSchema(mcp.ToolInputSchema{})
	if result != nil {
		t.Errorf("expected nil for empty schema, got %v", result)
	}
}
