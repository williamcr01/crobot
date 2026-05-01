package commands

import "testing"

func TestRegistrySourceDefaultsAndUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{Name: "help"})
	r.Register(Command{Name: "echoctl", Source: "plugin:echo"})

	native, ok := r.Get("help")
	if !ok || native.Source != "native" {
		t.Fatalf("native source = %q, ok=%v", native.Source, ok)
	}

	r.UnregisterBySource("plugin:echo")
	if r.Has("echoctl") {
		t.Fatal("plugin command was not unregistered")
	}
	if !r.Has("help") {
		t.Fatal("native command should remain registered")
	}
}

func TestRegistryUnregisterPluginCommands(t *testing.T) {
	r := NewRegistry()
	r.Register(Command{Name: "help"})
	r.Register(Command{Name: "echoctl", Source: "plugin:echo"})

	r.UnregisterPluginCommands()
	if r.Has("echoctl") {
		t.Fatal("plugin command was not unregistered")
	}
	if !r.Has("help") {
		t.Fatal("native command should remain registered")
	}
}
