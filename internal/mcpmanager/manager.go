// Package mcpmanager manages MCP server connections, lazy initialization,
// proxy tool operations, direct tool registration, and metadata caching.
package mcpmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"crobot/internal/mcpclient"
	"crobot/internal/mcpconfig"
	"crobot/internal/tools"
)

// Manager owns the configured MCP servers and exposes them as Crobot tools.
type Manager struct {
	cfg     *mcpconfig.Config
	cache   *cache
	clients map[string]*mcpclient.Client
	mu      sync.Mutex
	closed  bool
}

// New creates a Manager from the given config.
func New(cfg *mcpconfig.Config, cachePath string) *Manager {
	return &Manager{
		cfg:     cfg,
		cache:   newCache(cachePath),
		clients: make(map[string]*mcpclient.Client),
	}
}

// RegisterTools registers the MCP proxy tool and any direct tools.
func (m *Manager) RegisterTools(reg *tools.Registry) {
	// Load the cache so direct tools can be registered from it.
	m.cache.load()

	// Register proxy tool unless disabled.
	if !m.cfg.Settings.DisableProxyTool {
		reg.Register(m.proxyTool())
	}

	// Register direct tools.
	for name, srv := range m.cfg.MCPServers {
		dt := mcpconfig.ParseDirectTools(srv.DirectTools)
		if !dt.All && len(dt.Names) == 0 {
			// Also check global default.
			gdt := mcpconfig.ParseDirectTools(m.cfg.Settings.DirectTools)
			if !gdt.All && len(gdt.Names) == 0 {
				continue
			}
			dt = gdt
		}
		m.registerDirectTools(reg, name, srv, dt)
	}
}

// Close disconnects all active MCP clients.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for name, c := range m.clients {
		if err := c.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: closing MCP client %q: %v\n", name, err)
		}
	}
	m.clients = nil
	return nil
}

// ---------------------------------------------------------------------------
// Proxy tool operations
// ---------------------------------------------------------------------------

// Status returns MCP status information.
func (m *Manager) Status(ctx context.Context) (any, error) {
	servers := make([]any, 0, len(m.cfg.MCPServers))
	for name, srv := range m.cfg.MCPServers {
		entry := map[string]any{
			"server":     name,
			"command":    srv.Command,
			"lifecycle":  srv.Lifecycle,
			"connected":  m.isConnected(name),
			"cachedTools": 0,
		}
		if ct := m.cache.getTools(name); ct != nil {
			entry["cachedTools"] = len(ct)
		}
		servers = append(servers, entry)
	}
	return map[string]any{
		"enabled":     true,
		"proxyTool":   !m.cfg.Settings.DisableProxyTool,
		"configPath":  configPathOrWarning(),
		"serverCount": len(servers),
		"servers":     servers,
	}, nil
}

// ListServerTools lists tools for a specific server.
func (m *Manager) ListServerTools(ctx context.Context, server string) (any, error) {
	tools, err := m.ensureTools(ctx, server)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"server": server,
		"tools":  tools,
	}, nil
}

// SearchTools searches cached MCP tools by query.
func (m *Manager) SearchTools(ctx context.Context, query string) (any, error) {
	query = strings.ToLower(query)
	var results []any
	for name, srv := range m.cfg.MCPServers {
		exclude := makeSet(srv.ExcludeTools)
		tools := m.cache.getTools(name)
		if tools == nil {
			continue
		}
		for _, t := range tools {
			if exclude[t.Name] {
				continue
			}
			dl := strings.ToLower(t.Description)
			nl := strings.ToLower(t.Name)
			if strings.Contains(nl, query) || strings.Contains(dl, query) {
				results = append(results, map[string]any{
					"name":        t.DisplayName,
					"description": t.Description,
				})
			}
		}
	}
	return map[string]any{
		"query":   query,
		"results": results,
	}, nil
}

// DescribeTool returns full metadata for an MCP tool.
func (m *Manager) DescribeTool(ctx context.Context, displayName string) (any, error) {
	server, toolName, err := parseDisplayName(displayName)
	if err != nil {
		return nil, err
	}
	tools := m.cache.getTools(server)
	if tools == nil {
		return nil, fmt.Errorf("server %q not found or not cached", server)
	}
	for _, t := range tools {
		if t.Name == toolName {
			return map[string]any{
				"name":        t.DisplayName,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			}, nil
		}
	}
	return nil, fmt.Errorf("tool %q not found on server %q", toolName, server)
}

// CallTool invokes an MCP tool.
func (m *Manager) CallTool(ctx context.Context, displayName string, args map[string]any) (any, error) {
	server, toolName, err := parseDisplayName(displayName)
	if err != nil {
		return nil, err
	}

	client, err := m.ensureConnected(ctx, server)
	if err != nil {
		return nil, err
	}

	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// proxyTool returns the mcp proxy tool definition.
func (m *Manager) proxyTool() tools.Tool {
	return tools.Tool{
		Name:        "mcp",
		Source:      "mcp",
		Description: "Interact with MCP (Model Context Protocol) servers. Use to discover, describe, and call MCP tools.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"server": map[string]any{
					"type":        "string",
					"description": "List tools for a specific MCP server by name",
				},
				"search": map[string]any{
					"type":        "string",
					"description": "Search cached MCP tools by keyword",
				},
				"describe": map[string]any{
					"type":        "string",
					"description": "Describe an MCP tool (format: server_toolName)",
				},
				"tool": map[string]any{
					"type":        "string",
					"description": "Name of the MCP tool to call (format: server_toolName)",
				},
				"args": map[string]any{
					"type":        "object",
					"description": "Arguments to pass to the MCP tool",
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			return m.handleProxyCall(ctx, args)
		},
	}
}

func (m *Manager) handleProxyCall(ctx context.Context, args map[string]any) (any, error) {
	if displayName, ok := stringArg(args, "tool"); ok && displayName != "" {
		toolArgs, _ := args["args"].(map[string]any)
		return m.CallTool(ctx, displayName, toolArgs)
	}

	if desc, ok := stringArg(args, "describe"); ok && desc != "" {
		return m.DescribeTool(ctx, desc)
	}

	if query, ok := stringArg(args, "search"); ok && query != "" {
		return m.SearchTools(ctx, query)
	}

	if server, ok := stringArg(args, "server"); ok && server != "" {
		return m.ListServerTools(ctx, server)
	}

	return m.Status(ctx)
}

func (m *Manager) registerDirectTools(reg *tools.Registry, serverName string, srv mcpconfig.ServerConfig, dt mcpconfig.DirectToolsMode) {
	exclude := makeSet(srv.ExcludeTools)

	cachedTools := m.cache.getTools(serverName)
	if cachedTools == nil {
		fmt.Fprintf(os.Stderr, "warning: MCP direct tools for %q: no cached metadata, run /mcp reconnect %s first\n", serverName, serverName)
		return
	}

	for _, t := range cachedTools {
		if exclude[t.Name] {
			continue
		}
		if !dt.All {
			found := false
			for _, n := range dt.Names {
				if n == t.Name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		crobotName := "mcp__" + serverName + "__" + t.Name
		displayName := t.DisplayName
		schema := t.InputSchema
		reg.Register(tools.Tool{
			Name:        crobotName,
			Description: t.Description + " (MCP server: " + serverName + ")",
			InputSchema: schema,
			Source:      "mcp:" + serverName,
			Execute: func(ctx context.Context, args map[string]any) (any, error) {
				return m.CallTool(ctx, displayName, args)
			},
		})
	}
}

// ensureConnected gets or creates a connected client for a server.
func (m *Manager) ensureConnected(ctx context.Context, server string) (*mcpclient.Client, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("MCP manager is closed")
	}

	if c, ok := m.clients[server]; ok {
		m.mu.Unlock()
		return c, nil
	}
	m.mu.Unlock()

	srv, ok := m.cfg.MCPServers[server]
	if !ok {
		return nil, fmt.Errorf("MCP server %q is not configured", server)
	}

	client, err := mcpclient.NewStdioClient(server, srv)
	if err != nil {
		return nil, fmt.Errorf("starting server %q: %w", server, err)
	}

	if err := client.Initialize(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("initializing server %q: %w", server, err)
	}

	m.mu.Lock()
	if c, ok := m.clients[server]; ok {
		// Race: another goroutine connected first.
		m.mu.Unlock()
		client.Close()
		return c, nil
	}
	m.clients[server] = client
	m.mu.Unlock()

	return client, nil
}

// ensureTools returns cached tools for a server, connecting if needed.
func (m *Manager) ensureTools(ctx context.Context, server string) ([]mcpclient.ToolMetadata, error) {
	tools := m.cache.getTools(server)
	if tools != nil {
		return tools, nil
	}
	client, err := m.ensureConnected(ctx, server)
	if err != nil {
		return nil, err
	}
	tools, err = client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	m.cache.setTools(server, tools)
	return tools, nil
}

func (m *Manager) isConnected(server string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.clients[server]
	return ok
}

// ---------------------------------------------------------------------------
// Tool name parsing
// ---------------------------------------------------------------------------

func parseDisplayName(displayName string) (server, toolName string, err error) {
	idx := strings.Index(displayName, "_")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid MCP tool name %q (expected format: server_toolName)", displayName)
	}
	return displayName[:idx], displayName[idx+1:], nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func stringArg(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func makeSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func configPathOrWarning() string {
	p, err := mcpconfig.ConfigPath()
	if err != nil {
		return "unknown"
	}
	return p
}

// ---------------------------------------------------------------------------
// Metadata cache
// ---------------------------------------------------------------------------

type cache struct {
	path  string
	data  cacheData
	mu    sync.Mutex
	loaded bool
}

type cacheData struct {
	Servers map[string]cacheServer `json:"servers"`
}

type cacheServer struct {
	Tools []cachedTool `json:"tools"`
}

type cachedTool struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func newCache(path string) *cache {
	return &cache{path: path}
}

func (c *cache) load() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	c.loaded = true
	c.data.Servers = make(map[string]cacheServer)
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &c.data)
	if c.data.Servers == nil {
		c.data.Servers = make(map[string]cacheServer)
	}
}

func (c *cache) getTools(server string) []mcpclient.ToolMetadata {
	c.load()
	c.mu.Lock()
	defer c.mu.Unlock()
	srv, ok := c.data.Servers[server]
	if !ok || len(srv.Tools) == 0 {
		return nil
	}
	out := make([]mcpclient.ToolMetadata, len(srv.Tools))
	for i, t := range srv.Tools {
		out[i] = mcpclient.ToolMetadata{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Description: t.Description,
			InputSchema: t.InputSchema,
			ServerName:  server,
		}
	}
	return out
}

func (c *cache) setTools(server string, tools []mcpclient.ToolMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ct := make([]cachedTool, len(tools))
	for i, t := range tools {
		ct[i] = cachedTool{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	if c.data.Servers == nil {
		c.data.Servers = make(map[string]cacheServer)
	}
	c.data.Servers[server] = cacheServer{Tools: ct}
	c.writeLocked()
}

func (c *cache) writeLocked() {
	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = os.WriteFile(c.path, data, 0o644)
}
