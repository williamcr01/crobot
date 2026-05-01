package plugins

import (
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	ABIVersion = 1

	HookPrePrompt      = "pre_prompt"
	HookPostResponse   = "post_response"
	HookPreToolCall    = "pre_tool_call"
	HookPostToolResult = "post_tool_result"
	HookOnEvent        = "on_event"
)

// Manifest describes a WASM plugin and the capabilities it exposes.
type Manifest struct {
	ABIVersion  int               `json:"abi_version"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Tools       []ToolManifest    `json:"tools"`
	Hooks       []string          `json:"hooks"`
	Commands    []CommandManifest `json:"commands"`
}

type ToolManifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type CommandManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Args        string `json:"args"`
}

type Plugin struct {
	Manifest  Manifest
	FilePath  string
	Runtime   wazero.Runtime
	Module    api.Module
	Functions FunctionTable
	Timeout   TimeoutConfig
	mu        sync.Mutex
}

type FunctionTable struct {
	Malloc api.Function
	Free   api.Function

	Describe api.Function

	Execute        api.Function
	PrePrompt      api.Function
	PostResponse   api.Function
	PreToolCall    api.Function
	PostToolResult api.Function
	OnEvent        api.Function
	ExecuteCommand api.Function
}

type TimeoutConfig struct {
	Describe time.Duration
	Hook     time.Duration
	Tool     time.Duration
	Command  time.Duration
	Event    time.Duration
}

func defaultTimeouts() TimeoutConfig {
	return TimeoutConfig{
		Describe: 2 * time.Second,
		Hook:     2 * time.Second,
		Tool:     30 * time.Second,
		Command:  10 * time.Second,
		Event:    500 * time.Millisecond,
	}
}

type PluginInfo struct {
	Name        string
	Version     string
	Description string
	FilePath    string
	Tools       []string
	Hooks       []string
	Commands    []string
	Permissions []string
}

type LoadError struct {
	Path  string
	Error string
}

type ExecuteOutput struct {
	Content any    `json:"content"`
	Error   string `json:"error"`
}

type CommandOutput struct {
	Output string `json:"output"`
	Error  string `json:"error"`
}
