package provider

import (
	"testing"
)

func TestCreateOpenRouter(t *testing.T) {
	prov, err := Create("openrouter", "sk-or-v1-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "openrouter" {
		t.Errorf("expected name openrouter, got %s", prov.Name())
	}
}

func TestCreateOpenAI(t *testing.T) {
	prov, err := Create("openai", "sk-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	if prov.Name() != "openai" {
		t.Errorf("expected name openai, got %s", prov.Name())
	}
}

func TestCreateUnsupported(t *testing.T) {
	prov, err := Create("nonexistent", "key")
	if err == nil {
		t.Fatal("expected error")
	}
	if prov != nil {
		t.Errorf("expected nil provider, got %v", prov)
	}
	if err.Error() != "unsupported provider: nonexistent" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestErrUnsupportedProvider(t *testing.T) {
	err := ErrUnsupportedProvider("foo")
	if err.Error() != "unsupported provider: foo" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestErrStreamClosed(t *testing.T) {
	if ErrStreamClosed.Error() != "stream closed" {
		t.Errorf("unexpected error: %v", ErrStreamClosed)
	}
}

func TestMessageToolCallID(t *testing.T) {
	m := Message{Role: "tool", ToolCallID: "call_abc123", Content: "some result"}
	if m.ToolCallID != "call_abc123" {
		t.Errorf("expected call_abc123, got %s", m.ToolCallID)
	}
}

func TestParseToolArgs(t *testing.T) {
	// Valid JSON.
	args := parseToolArgs(`{"key": "value", "num": 42}`)
	if args["key"] != "value" {
		t.Errorf("expected value, got %v", args["key"])
	}
	if args["num"] != float64(42) {
		t.Errorf("expected 42, got %v", args["num"])
	}

	// Empty string.
	if parsed := parseToolArgs(""); parsed != nil {
		t.Errorf("expected nil, got %v", parsed)
	}

	// Invalid JSON.
	parsed := parseToolArgs("not json")
	if parsed["raw"] != "not json" {
		t.Errorf("expected raw fallback, got %v", parsed)
	}
}
