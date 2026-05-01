package plugins

import (
	"context"
	"strings"
	"testing"

	"crobot/internal/commands"
	"crobot/internal/config"
	"crobot/internal/provider"
	"crobot/internal/tools"
)

func TestValidateManifest(t *testing.T) {
	valid := Manifest{ABIVersion: ABIVersion, Name: "echo"}
	if err := validateManifest(valid); err != nil {
		t.Fatalf("valid manifest failed: %v", err)
	}

	cases := []struct {
		name     string
		manifest Manifest
	}{
		{"abi", Manifest{ABIVersion: 99, Name: "echo"}},
		{"name", Manifest{ABIVersion: ABIVersion}},
		{"duplicate tool", Manifest{ABIVersion: ABIVersion, Name: "echo", Tools: []ToolManifest{{Name: "x"}, {Name: "x"}}}},
		{"duplicate command", Manifest{ABIVersion: ABIVersion, Name: "echo", Commands: []CommandManifest{{Name: "x"}, {Name: "x"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateManifest(tc.manifest); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestManagerSummaryNoPluginsIncludesDirectoriesAndErrors(t *testing.T) {
	m := NewManager(config.PluginConfig{Directories: []string{"~/.crobot/plugins"}}, tools.NewRegistry(), commands.NewRegistry(), nil)
	m.addLoadError("bad.wasm", assertErr("missing required export describe"))

	summary := m.Summary()
	for _, want := range []string{"No plugins loaded.", "Plugin directories:", "~/.crobot/plugins", "Plugin load errors:", "bad.wasm: missing required export describe"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestManagerDrainMessages(t *testing.T) {
	m := NewManager(config.PluginConfig{}, tools.NewRegistry(), commands.NewRegistry(), nil)
	m.messageQueue = []provider.Message{{Role: "system", Content: "queued"}}

	msgs := m.DrainMessages()
	if len(msgs) != 1 || msgs[0].Content != "queued" {
		t.Fatalf("DrainMessages() = %#v", msgs)
	}
	if msgs := m.DrainMessages(); len(msgs) != 0 {
		t.Fatalf("second DrainMessages() = %#v, want empty", msgs)
	}
}

func TestManagerRegisterPluginCommandCollisionSkipsPluginCommand(t *testing.T) {
	toolReg := tools.NewRegistry()
	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.Command{Name: "help", Source: "native"})
	m := NewManager(config.PluginConfig{}, toolReg, cmdReg, nil)

	p := &Plugin{Manifest: Manifest{ABIVersion: ABIVersion, Name: "echo", Commands: []CommandManifest{{Name: "help"}}}, FilePath: "echo.wasm"}
	if err := m.registerPlugin(p); err != nil {
		t.Fatalf("registerPlugin() error = %v", err)
	}
	cmd, ok := cmdReg.Get("help")
	if !ok || cmd.Source != "native" {
		t.Fatalf("native command replaced: %#v ok=%v", cmd, ok)
	}
	if len(m.loadErrors) != 1 || !strings.Contains(m.loadErrors[0].Error, "already registered") {
		t.Fatalf("loadErrors = %#v", m.loadErrors)
	}
}

func TestManagerRegisterPluginToolCollisionFails(t *testing.T) {
	toolReg := tools.NewRegistry()
	cmdReg := commands.NewRegistry()
	toolReg.Register(tools.Tool{Name: "file_read", Source: "native"})
	m := NewManager(config.PluginConfig{}, toolReg, cmdReg, nil)

	p := &Plugin{Manifest: Manifest{ABIVersion: ABIVersion, Name: "echo", Tools: []ToolManifest{{Name: "file_read"}}}}
	if err := m.registerPlugin(p); err == nil {
		t.Fatal("expected tool collision error")
	}
}

func TestManagerReloadUnregistersPluginSourcesWhenDisabled(t *testing.T) {
	toolReg := tools.NewRegistry()
	cmdReg := commands.NewRegistry()
	toolReg.Register(tools.Tool{Name: "native"})
	toolReg.Register(tools.Tool{Name: "plugin_tool", Source: "plugin:echo"})
	cmdReg.Register(commands.Command{Name: "help"})
	cmdReg.Register(commands.Command{Name: "echoctl", Source: "plugin:echo"})
	m := NewManager(config.PluginConfig{Enabled: false}, toolReg, cmdReg, nil)
	m.plugins = []*Plugin{{Manifest: Manifest{Name: "echo"}}}

	if err := m.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if toolReg.Has("plugin_tool") || cmdReg.Has("echoctl") {
		t.Fatal("plugin sources were not unregistered")
	}
	if !toolReg.Has("native") || !cmdReg.Has("help") {
		t.Fatal("native registrations should remain")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
