package tools

import (
	"context"
	"testing"
)

func TestRegistrySourceDefaultsAndUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{Name: "native", Execute: func(ctx context.Context, args map[string]any) (any, error) { return "ok", nil }})
	r.Register(Tool{Name: "plugin_tool", Source: "plugin:echo", Execute: func(ctx context.Context, args map[string]any) (any, error) { return "ok", nil }})

	native, ok := r.Get("native")
	if !ok || native.Source != "native" {
		t.Fatalf("native source = %q, ok=%v", native.Source, ok)
	}

	r.UnregisterBySource("plugin:echo")
	if r.Has("plugin_tool") {
		t.Fatal("plugin tool was not unregistered")
	}
	if !r.Has("native") {
		t.Fatal("native tool should remain registered")
	}
}

func TestRegistryUnregisterPluginTools(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{Name: "native"})
	r.Register(Tool{Name: "plugin_tool", Source: "plugin:echo"})

	r.UnregisterPluginTools()
	if r.Has("plugin_tool") {
		t.Fatal("plugin tool was not unregistered")
	}
	if !r.Has("native") {
		t.Fatal("native tool should remain registered")
	}
}
