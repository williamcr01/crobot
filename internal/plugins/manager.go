package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"crobot/internal/agent"
	"crobot/internal/commands"
	"crobot/internal/config"
	"crobot/internal/events"
	"crobot/internal/provider"
	"crobot/internal/tools"

	"github.com/tetratelabs/wazero/api"
)

type Manager struct {
	plugins      []*Plugin
	toolReg      *tools.Registry
	cmdReg       *commands.Registry
	events       *events.Logger
	cfg          config.PluginConfig
	loadErrors   []LoadError
	messageQueue []provider.Message
	mu           sync.RWMutex
}

func NewManager(cfg config.PluginConfig, toolReg *tools.Registry, cmdReg *commands.Registry, ev *events.Logger) *Manager {
	return &Manager{cfg: cfg, toolReg: toolReg, cmdReg: cmdReg, events: ev}
}

func (m *Manager) LoadAll(ctx context.Context) error {
	if !m.cfg.Enabled {
		return nil
	}
	m.mu.Lock()
	m.loadErrors = nil
	m.mu.Unlock()

	paths := m.discover()
	seen := map[string]bool{}
	for _, path := range paths {
		p, err := m.loadOne(ctx, path)
		if err != nil {
			m.addLoadError(path, err)
			continue
		}
		if seen[p.Manifest.Name] || m.hasPlugin(p.Manifest.Name) {
			p.Module.Close(ctx)
			p.Runtime.Close(ctx)
			m.addLoadError(path, fmt.Errorf("duplicate plugin name %q", p.Manifest.Name))
			continue
		}
		seen[p.Manifest.Name] = true
		if err := m.registerPlugin(p); err != nil {
			p.Module.Close(ctx)
			p.Runtime.Close(ctx)
			m.addLoadError(path, err)
			continue
		}
		m.mu.Lock()
		m.plugins = append(m.plugins, p)
		m.mu.Unlock()
		if m.events != nil {
			m.events.PluginLoad(p.Manifest.Name, p.Manifest.Version, "loaded")
		}
	}
	return nil
}

func (m *Manager) Reload(ctx context.Context) error {
	m.UnloadAll()
	return m.LoadAll(ctx)
}

func (m *Manager) UnloadAll() {
	m.mu.Lock()
	plugins := append([]*Plugin(nil), m.plugins...)
	m.plugins = nil
	m.messageQueue = nil
	m.mu.Unlock()
	for _, p := range plugins {
		m.unregisterPlugin(p.Manifest.Name)
		if p.Module != nil {
			_ = p.Module.Close(context.Background())
		}
		if p.Runtime != nil {
			_ = p.Runtime.Close(context.Background())
		}
		if m.events != nil {
			m.events.PluginLoad(p.Manifest.Name, p.Manifest.Version, "unloaded")
		}
	}
}

func (m *Manager) UnloadPlugin(name string) error {
	m.mu.Lock()
	idx := -1
	var p *Plugin
	for i, candidate := range m.plugins {
		if candidate.Manifest.Name == name {
			idx = i
			p = candidate
			break
		}
	}
	if p == nil {
		m.mu.Unlock()
		return fmt.Errorf("plugin not loaded: %s", name)
	}
	m.plugins = append(m.plugins[:idx], m.plugins[idx+1:]...)
	m.mu.Unlock()

	m.unregisterPlugin(name)
	if p.Module != nil {
		_ = p.Module.Close(context.Background())
	}
	if p.Runtime != nil {
		_ = p.Runtime.Close(context.Background())
	}
	if m.events != nil {
		m.events.PluginLoad(p.Manifest.Name, p.Manifest.Version, "unloaded")
	}
	return nil
}

func (m *Manager) DrainMessages() []provider.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]provider.Message(nil), m.messageQueue...)
	m.messageQueue = nil
	return out
}

func (m *Manager) Plugins() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]PluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, m.infoFor(p))
	}
	return out
}

func (m *Manager) HasHook(hook string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.plugins {
		if hasString(p.Manifest.Hooks, hook) {
			return true
		}
	}
	return false
}

func (m *Manager) CallPrePrompt(systemPrompt string, messages []provider.Message) (string, []provider.Message, error) {
	input := struct {
		SystemPrompt string             `json:"system_prompt"`
		Messages     []provider.Message `json:"messages"`
	}{systemPrompt, messages}
	out := input
	for _, p := range m.snapshot() {
		if p.Functions.PrePrompt == nil {
			continue
		}
		if err := m.callHook(p, p.Functions.PrePrompt, input, &out); err != nil {
			m.logPluginError("pre_prompt", p, err)
			continue
		}
		input = out
	}
	return input.SystemPrompt, input.Messages, nil
}

func (m *Manager) CallPostResponse(resp *agent.Result) (*agent.Result, error) {
	out := resp
	for _, p := range m.snapshot() {
		if p.Functions.PostResponse == nil {
			continue
		}
		var next agent.Result
		if err := m.callHook(p, p.Functions.PostResponse, out, &next); err != nil {
			m.logPluginError("post_response", p, err)
			continue
		}
		out = &next
	}
	return out, nil
}

func (m *Manager) CallPreToolCall(name string, args map[string]any) (string, map[string]any, bool, error) {
	input := struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	}{name, args}
	for _, p := range m.snapshot() {
		if p.Functions.PreToolCall == nil {
			continue
		}
		var out struct {
			Name string         `json:"name"`
			Args map[string]any `json:"args"`
			Skip bool           `json:"skip"`
		}
		if err := m.callHook(p, p.Functions.PreToolCall, input, &out); err != nil {
			m.logPluginError("pre_tool_call", p, err)
			continue
		}
		if out.Name != "" {
			input.Name = out.Name
		}
		if out.Args != nil {
			input.Args = out.Args
		}
		if out.Skip {
			return input.Name, input.Args, true, nil
		}
	}
	return input.Name, input.Args, false, nil
}

func (m *Manager) CallPostToolResult(name string, args map[string]any, result any) (any, error) {
	input := struct {
		Name   string         `json:"name"`
		Args   map[string]any `json:"args"`
		Result any            `json:"result"`
	}{name, args, result}
	for _, p := range m.snapshot() {
		if p.Functions.PostToolResult == nil {
			continue
		}
		var out struct {
			Result any `json:"result"`
		}
		if err := m.callHook(p, p.Functions.PostToolResult, input, &out); err != nil {
			m.logPluginError("post_tool_result", p, err)
			continue
		}
		if out.Result != nil {
			input.Result = out.Result
		}
	}
	return input.Result, nil
}

func (m *Manager) CallOnEvent(ev agent.Event) error {
	payload := eventDTO(ev)
	for _, p := range m.snapshot() {
		if p.Functions.OnEvent == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Event)
		p.mu.Lock()
		err := callJSONIn(ctx, p, p.Functions.OnEvent, payload)
		p.mu.Unlock()
		cancel()
		if err != nil {
			m.logPluginError("on_event", p, err)
		}
	}
	return nil
}

func (m *Manager) CallExecuteCommand(pluginName, cmd string, args []string) (string, error) {
	p := m.find(pluginName)
	if p == nil {
		return "", fmt.Errorf("plugin not loaded: %s", pluginName)
	}
	if p.Functions.ExecuteCommand == nil {
		return "", fmt.Errorf("plugin %s does not implement commands", pluginName)
	}
	var out CommandOutput
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Command)
	defer cancel()
	p.mu.Lock()
	err := callStringStringJSONOut(ctx, p, p.Functions.ExecuteCommand, cmd, args, &out)
	p.mu.Unlock()
	if err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("%s", out.Error)
	}
	return out.Output, nil
}

func (m *Manager) Summary() string {
	infos := m.Plugins()
	m.mu.RLock()
	loadErrors := append([]LoadError(nil), m.loadErrors...)
	dirs := append([]string(nil), m.cfg.Directories...)
	m.mu.RUnlock()
	var b strings.Builder
	if len(infos) == 0 {
		b.WriteString("No plugins loaded.\n")
		b.WriteString("Plugin directories:\n")
		for _, dir := range dirs {
			b.WriteString("  ")
			b.WriteString(dir)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("Plugins:\n")
		for _, info := range infos {
			b.WriteString(fmt.Sprintf("  %s %s\n", info.Name, info.Version))
			b.WriteString(fmt.Sprintf("    path: %s\n", info.FilePath))
			if len(info.Tools) > 0 {
				b.WriteString(fmt.Sprintf("    tools: %s\n", strings.Join(info.Tools, ", ")))
			}
			if len(info.Hooks) > 0 {
				b.WriteString(fmt.Sprintf("    hooks: %s\n", strings.Join(info.Hooks, ", ")))
			}
			if len(info.Commands) > 0 {
				b.WriteString(fmt.Sprintf("    commands: %s\n", strings.Join(info.Commands, ", ")))
			}
			b.WriteString(fmt.Sprintf("    permissions: %s\n", strings.Join(info.Permissions, ", ")))
		}
	}
	if len(loadErrors) > 0 {
		b.WriteString("\nPlugin load errors:\n")
		for _, err := range loadErrors {
			b.WriteString(fmt.Sprintf("  %s: %s\n", err.Path, err.Error))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Manager) executePluginTool(ctx context.Context, pluginName string, name string, args map[string]any) (any, error) {
	p := m.find(pluginName)
	if p == nil {
		return nil, fmt.Errorf("plugin not loaded: %s", pluginName)
	}
	if p.Functions.Execute == nil {
		return nil, fmt.Errorf("plugin %s does not implement execute", pluginName)
	}
	var out ExecuteOutput
	ctx, cancel := context.WithTimeout(ctx, p.Timeout.Tool)
	defer cancel()
	p.mu.Lock()
	err := callStringStringJSONOut(ctx, p, p.Functions.Execute, name, args, &out)
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("%s", out.Error)
	}
	return out.Content, nil
}

func (m *Manager) discover() []string {
	var paths []string
	for _, dir := range m.cfg.Directories {
		expanded := expandPath(dir)
		matches, err := filepath.Glob(filepath.Join(expanded, "*.wasm"))
		if err != nil {
			m.addLoadError(dir, err)
			continue
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	return paths
}

func (m *Manager) loadOne(ctx context.Context, path string) (*Plugin, error) {
	rt, err := newRuntime(ctx)
	if err != nil {
		return nil, err
	}
	host, err := buildHostModule(ctx, rt, m)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}
	mod, err := instantiateModule(ctx, rt, path, host)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}
	p := &Plugin{FilePath: path, Runtime: rt, Module: mod, Timeout: defaultTimeouts()}
	p.Functions = functionTable(mod)
	manifest, err := m.describe(ctx, p)
	if err != nil {
		mod.Close(ctx)
		rt.Close(ctx)
		return nil, err
	}
	if err := validateManifest(manifest); err != nil {
		mod.Close(ctx)
		rt.Close(ctx)
		return nil, err
	}
	p.Manifest = manifest
	return p, nil
}

func functionTable(mod api.Module) FunctionTable {
	return FunctionTable{
		Malloc:         mod.ExportedFunction("malloc"),
		Free:           mod.ExportedFunction("free"),
		Describe:       mod.ExportedFunction("describe"),
		Execute:        mod.ExportedFunction("execute"),
		PrePrompt:      mod.ExportedFunction("pre_prompt"),
		PostResponse:   mod.ExportedFunction("post_response"),
		PreToolCall:    mod.ExportedFunction("pre_tool_call"),
		PostToolResult: mod.ExportedFunction("post_tool_result"),
		OnEvent:        mod.ExportedFunction("on_event"),
		ExecuteCommand: mod.ExportedFunction("execute_command"),
	}
}

func (m *Manager) describe(ctx context.Context, p *Plugin) (Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, p.Timeout.Describe)
	defer cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	results, err := p.Functions.Describe.Call(ctx)
	if err != nil {
		return Manifest{}, err
	}
	if len(results) == 0 {
		return Manifest{}, fmt.Errorf("describe returned no result")
	}
	ptr, length := unpackPtrLen(results[0])
	data, err := readBytes(ctx, p.Module, p.Functions.Free, ptr, length)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.ABIVersion != ABIVersion {
		return fmt.Errorf("unsupported abi_version %d", manifest.ABIVersion)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return fmt.Errorf("manifest name is required")
	}
	toolNames := map[string]bool{}
	for _, tool := range manifest.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return fmt.Errorf("tool name is required")
		}
		if toolNames[tool.Name] {
			return fmt.Errorf("duplicate tool name %q", tool.Name)
		}
		toolNames[tool.Name] = true
	}
	cmdNames := map[string]bool{}
	for _, cmd := range manifest.Commands {
		if strings.TrimSpace(cmd.Name) == "" {
			return fmt.Errorf("command name is required")
		}
		if cmdNames[cmd.Name] {
			return fmt.Errorf("duplicate command name %q", cmd.Name)
		}
		cmdNames[cmd.Name] = true
	}
	return nil
}

func (m *Manager) registerPlugin(p *Plugin) error {
	source := "plugin:" + p.Manifest.Name
	for _, tool := range p.Manifest.Tools {
		if existing, ok := m.toolReg.Get(tool.Name); ok && existing.Source != source {
			return fmt.Errorf("tool %q already registered by %s", tool.Name, existing.Source)
		}
	}
	for _, tool := range p.Manifest.Tools {
		toolName := tool.Name
		m.toolReg.Register(tools.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			Source:      source,
			Execute: func(ctx context.Context, args map[string]any) (any, error) {
				return m.executePluginTool(ctx, p.Manifest.Name, toolName, args)
			},
		})
	}
	for _, cmd := range p.Manifest.Commands {
		if existing, ok := m.cmdReg.Get(cmd.Name); ok && existing.Source != source {
			m.addLoadError(p.FilePath, fmt.Errorf("command %q already registered by %s", cmd.Name, existing.Source))
			continue
		}
		cmdName := cmd.Name
		m.cmdReg.Register(commands.Command{
			Name:        cmd.Name,
			Description: cmd.Description,
			Args:        cmd.Args,
			Source:      source,
			Handler: func(args []string) (string, error) {
				return m.CallExecuteCommand(p.Manifest.Name, cmdName, args)
			},
		})
	}
	return nil
}

func (m *Manager) unregisterPlugin(name string) {
	source := "plugin:" + name
	m.toolReg.UnregisterBySource(source)
	m.cmdReg.UnregisterBySource(source)
}

func (m *Manager) callHook(p *Plugin, fn api.Function, input any, output any) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Hook)
	defer cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	return callJSONInJSONOut(ctx, p, fn, input, output)
}

func (m *Manager) snapshot() []*Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]*Plugin(nil), m.plugins...)
}

func (m *Manager) find(name string) *Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.plugins {
		if p.Manifest.Name == name {
			return p
		}
	}
	return nil
}

func (m *Manager) hasPlugin(name string) bool {
	return m.find(name) != nil
}

func (m *Manager) infoFor(p *Plugin) PluginInfo {
	var toolsList []string
	for _, t := range p.Manifest.Tools {
		toolsList = append(toolsList, t.Name)
	}
	var commandsList []string
	for _, c := range p.Manifest.Commands {
		commandsList = append(commandsList, c.Name)
	}
	return PluginInfo{
		Name:        p.Manifest.Name,
		Version:     p.Manifest.Version,
		Description: p.Manifest.Description,
		FilePath:    p.FilePath,
		Tools:       toolsList,
		Hooks:       append([]string(nil), p.Manifest.Hooks...),
		Commands:    commandsList,
		Permissions: m.permissionsFor(p),
	}
}

func (m *Manager) addLoadError(path string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadErrors = append(m.loadErrors, LoadError{Path: path, Error: err.Error()})
	if m.events != nil {
		m.events.Error("plugin load", err)
	}
}

func (m *Manager) logPluginError(context string, p *Plugin, err error) {
	if m.events != nil {
		m.events.Error("plugin "+p.Manifest.Name+" "+context, err)
	}
}

func eventDTO(ev agent.Event) map[string]any {
	data := map[string]any{}
	if ev.MessageStart != nil {
		data["role"] = ev.MessageStart.Role
		data["content"] = ev.MessageStart.Content
	}
	if ev.MessageEnd != nil {
		data["role"] = ev.MessageEnd.Role
		data["text"] = ev.MessageEnd.Text
		data["usage"] = ev.MessageEnd.Usage
	}
	if ev.TextDelta != "" {
		data["text_delta"] = ev.TextDelta
	}
	if ev.ReasoningDelta != "" {
		data["reasoning_delta"] = ev.ReasoningDelta
	}
	if ev.ToolCallStart != nil {
		data["tool_call"] = ev.ToolCallStart
	}
	if ev.ToolCallEnd != nil {
		data["tool_call"] = ev.ToolCallEnd
	}
	if ev.ToolExecStart != nil {
		data["tool_exec"] = ev.ToolExecStart
	}
	if ev.ToolExecResult != nil {
		data["tool_result"] = ev.ToolExecResult
	}
	if ev.TurnUsage != nil {
		data["usage"] = ev.TurnUsage
	}
	if ev.Error != nil {
		data["error"] = ev.Error.Error()
	}
	return map[string]any{"type": ev.Type, "data": data}
}

func expandPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
