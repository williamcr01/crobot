// Package mcpclient provides MCP client implementations and shared types.
package mcpclient

import (
	"context"
	"fmt"

	"crobot/internal/mcpconfig"

	sdkclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client connects to an MCP server, initializes the session, lists tools,
// and calls tools.
type Client struct {
	inner      *sdkclient.Client
	serverName string
}

// ToolMetadata describes an MCP tool.
type ToolMetadata struct {
	ServerName  string
	Name        string
	DisplayName string
	Description string
	InputSchema map[string]any
}

// CallResult is the result of an MCP tool call.
type CallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is one content item in an MCP tool result.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data any    `json:"data,omitempty"`
}

// NewStdioClient creates an MCP stdio client for a configured server.
func NewStdioClient(serverName string, cfg mcpconfig.ServerConfig) (*Client, error) {
	srvEnv := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		srvEnv = append(srvEnv, fmt.Sprintf("%s=%s", k, v))
	}

	inner, err := sdkclient.NewStdioMCPClient(cfg.Command, srvEnv, cfg.Args...)
	if err != nil {
		return nil, fmt.Errorf("server %q: failed to create stdio client: %w", serverName, err)
	}

	return &Client{
		inner:      inner,
		serverName: serverName,
	}, nil
}

// Initialize initializes the MCP connection. Safe to call multiple times;
// if already initialized, it is a no-op.
func (c *Client) Initialize(ctx context.Context) error {
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "crobot",
		Version: "1.0.0",
	}

	_, err := c.inner.Initialize(ctx, initReq)
	if err != nil {
		return fmt.Errorf("server %q: initialize failed: %w", c.serverName, err)
	}
	return nil
}

// ListTools returns the tools available on the server.
func (c *Client) ListTools(ctx context.Context) ([]ToolMetadata, error) {
	resp, err := c.inner.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("server %q: list tools failed: %w", c.serverName, err)
	}

	var tools []ToolMetadata
	for _, t := range resp.Tools {
		tools = append(tools, ToolMetadata{
			ServerName:  c.serverName,
			Name:        t.Name,
			DisplayName: c.serverName + "_" + t.Name,
			Description: t.Description,
			InputSchema: convertInputSchema(t.InputSchema),
		})
	}
	return tools, nil
}

// CallTool invokes a named tool on the server with the given arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.inner.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("server %q: call tool %q failed: %w", c.serverName, name, err)
	}

	return normalizeResult(result), nil
}

// Close shuts down the client.
func (c *Client) Close() error {
	return c.inner.Close()
}

// ServerName returns the configured server name.
func (c *Client) ServerName() string {
	return c.serverName
}

// normalizeResult converts the SDK's CallToolResult to our CallResult.
func normalizeResult(result *mcp.CallToolResult) *CallResult {
	out := &CallResult{
		IsError: result.IsError,
	}
	for _, content := range result.Content {
		switch ct := content.(type) {
		case mcp.TextContent:
			out.Content = append(out.Content, ContentBlock{
				Type: "text",
				Text: ct.Text,
			})
		default:
			out.Content = append(out.Content, ContentBlock{
				Type: "unknown",
				Data: content,
			})
		}
	}
	return out
}

// convertInputSchema converts the SDK's ToolInputSchema to a plain map.
func convertInputSchema(schema mcp.ToolInputSchema) map[string]any {
	if schema.Properties == nil && schema.Type == "" {
		return nil
	}
	out := map[string]any{}
	if schema.Type != "" {
		out["type"] = schema.Type
	}
	if schema.Properties != nil {
		out["properties"] = schema.Properties
	}
	if len(schema.Required) > 0 {
		out["required"] = schema.Required
	}
	return out
}
