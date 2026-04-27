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

func TestMessageExtractToolCallID(t *testing.T) {
	m := Message{Role: "tool", Content: "call_abc123: some result"}
	id := m.extractToolCallID()
	if id != "call_abc123" {
		t.Errorf("expected call_abc123, got %s", id)
	}

	m2 := Message{Role: "tool", Content: "no prefix"}
	id2 := m2.extractToolCallID()
	if id2 != "" {
		t.Errorf("expected empty, got %s", id2)
	}

	m3 := Message{Role: "tool", Content: "call_123: one: two"}
	id3 := m3.extractToolCallID()
	if id3 != "call_123" {
		t.Errorf("expected call_123, got %s", id3)
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
