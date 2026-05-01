package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"crobot/internal/provider"

	"github.com/tetratelabs/wazero/api"
)

type pluginCallDepthKey struct{}

const maxPluginCallDepth = 3

func (m *Manager) hostLog(ctx context.Context, mod api.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
	level := m.readGuestString(mod, levelPtr, levelLen)
	msg := m.readGuestString(mod, msgPtr, msgLen)
	if m.events == nil {
		return
	}
	if strings.EqualFold(level, "error") {
		m.events.Error("plugin", fmt.Errorf("%s", msg))
		return
	}
	m.events.PluginLoad(m.pluginNameForModule(mod), "", msg)
}

func (m *Manager) hostConfigGet(ctx context.Context, mod api.Module, keyPtr, keyLen uint32) uint64 {
	key := m.readGuestString(mod, keyPtr, keyLen)
	p := m.pluginForModule(mod)
	if !m.hasPermission(p, "config") && !m.hasPermission(p, "config:"+key) {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "permission denied: config"})
	}
	if key == "plugins.enabled" {
		return m.writeHostJSON(ctx, mod, map[string]any{"value": m.cfg.Enabled})
	}
	if key == "plugins.directories" {
		return m.writeHostJSON(ctx, mod, map[string]any{"value": m.cfg.Directories})
	}
	if key == "plugins.permissions" {
		return m.writeHostJSON(ctx, mod, map[string]any{"value": m.permissionsFor(p)})
	}
	return m.writeHostJSON(ctx, mod, map[string]any{"value": nil})
}

func (m *Manager) hostEnvGet(ctx context.Context, mod api.Module, keyPtr, keyLen uint32) uint64 {
	key := m.readGuestString(mod, keyPtr, keyLen)
	p := m.pluginForModule(mod)
	if !m.hasPermission(p, "env:"+key) {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "permission denied: env"})
	}
	return m.writeHostJSON(ctx, mod, map[string]any{"value": os.Getenv(key)})
}

func (m *Manager) hostFileRead(ctx context.Context, mod api.Module, pathPtr, pathLen uint32) uint64 {
	path := m.readGuestString(mod, pathPtr, pathLen)
	p := m.pluginForModule(mod)
	if !m.hasPermission(p, "file_read") {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "permission denied: file_read"})
	}
	result, err := m.toolReg.Execute(ctx, "file_read", map[string]any{"path": path})
	if err != nil {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": err.Error()})
	}
	return m.writeHostJSON(ctx, mod, map[string]any{"value": result})
}

func (m *Manager) hostFileWrite(ctx context.Context, mod api.Module, pathPtr, pathLen, contentPtr, contentLen uint32) {
	path := m.readGuestString(mod, pathPtr, pathLen)
	content := m.readGuestString(mod, contentPtr, contentLen)
	p := m.pluginForModule(mod)
	if !m.hasPermission(p, "file_write") {
		return
	}
	_, _ = m.toolReg.Execute(ctx, "file_write", map[string]any{"path": path, "content": content})
}

func (m *Manager) hostToolCall(ctx context.Context, mod api.Module, namePtr, nameLen, argsPtr, argsLen uint32) uint64 {
	p := m.pluginForModule(mod)
	if !m.hasPermission(p, "tool_call") {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "permission denied: tool_call"})
	}
	depth, _ := ctx.Value(pluginCallDepthKey{}).(int)
	if depth >= maxPluginCallDepth {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "max plugin call depth exceeded"})
	}
	name := m.readGuestString(mod, namePtr, nameLen)
	if name == "bash" && !m.hasPermission(p, "bash") {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "permission denied: bash"})
	}
	if t, ok := m.toolReg.Get(name); ok && strings.HasPrefix(t.Source, "plugin:") {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": "plugin tool recursion is not allowed"})
	}
	argsBytes, err := readBytesNoFree(mod, argsPtr, argsLen)
	if err != nil {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": err.Error()})
	}
	var args map[string]any
	if len(argsBytes) > 0 {
		if err := json.Unmarshal(argsBytes, &args); err != nil {
			return m.writeHostJSON(ctx, mod, map[string]any{"error": err.Error()})
		}
	}
	ctx = context.WithValue(ctx, pluginCallDepthKey{}, depth+1)
	result, err := m.toolReg.Execute(ctx, name, args)
	if err != nil {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": err.Error()})
	}
	return m.writeHostJSON(ctx, mod, map[string]any{"value": result})
}

func (m *Manager) hostSendMessage(ctx context.Context, mod api.Module, rolePtr, roleLen, contentPtr, contentLen uint32) {
	p := m.pluginForModule(mod)
	if !m.hasPermission(p, "send_message") {
		return
	}
	role := m.readGuestString(mod, rolePtr, roleLen)
	content := m.readGuestString(mod, contentPtr, contentLen)
	if role == "" {
		role = "system"
	}
	m.mu.Lock()
	m.messageQueue = append(m.messageQueue, provider.Message{Role: role, Content: content})
	m.mu.Unlock()
}

func (m *Manager) hostGetCWD(ctx context.Context, mod api.Module) uint64 {
	cwd, err := os.Getwd()
	if err != nil {
		return m.writeHostJSON(ctx, mod, map[string]any{"error": err.Error()})
	}
	return m.writeHostJSON(ctx, mod, map[string]any{"value": cwd})
}

func (m *Manager) readGuestString(mod api.Module, ptr, length uint32) string {
	data, err := readBytesNoFree(mod, ptr, length)
	if err != nil {
		return ""
	}
	return string(data)
}

func (m *Manager) writeHostJSON(ctx context.Context, mod api.Module, v any) uint64 {
	malloc := mod.ExportedFunction("malloc")
	if malloc == nil {
		return 0
	}
	ptr, length, err := writeJSON(ctx, mod, malloc, v)
	if err != nil {
		return 0
	}
	return packPtrLen(ptr, length)
}

func (m *Manager) pluginNameForModule(mod api.Module) string {
	if p := m.pluginForModule(mod); p != nil {
		return p.Manifest.Name
	}
	return "unknown"
}

func (m *Manager) pluginForModule(mod api.Module) *Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.plugins {
		if p.Module == mod {
			return p
		}
	}
	return nil
}

func (m *Manager) permissionsFor(p *Plugin) []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), m.cfg.Permissions...)
}

func (m *Manager) hasPermission(p *Plugin, perm string) bool {
	if p == nil {
		return false
	}
	for _, allowed := range m.cfg.Permissions {
		if allowed == perm {
			return true
		}
	}
	return false
}
