package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewModelHistory_NoFile(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)
	if h == nil {
		t.Fatal("expected non-nil history")
	}
	if len(h.Recent) != 0 {
		t.Fatalf("expected empty recent list, got %d", len(h.Recent))
	}
}

func TestNewModelHistory_LoadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)
	h.Record("openrouter", "model-a")
	h.Record("openrouter", "model-b")

	// Create a new instance to force reload from disk.
	h2 := NewModelHistory(dir)
	if len(h2.Recent) != 2 {
		t.Fatalf("expected 2 recent models, got %d", len(h2.Recent))
	}
	if h2.Recent[0] != "openrouter/model-b" {
		t.Fatalf("expected most recent model first, got %q", h2.Recent[0])
	}
	if h2.Recent[1] != "openrouter/model-a" {
		t.Fatalf("expected second model, got %q", h2.Recent[1])
	}
}

func TestRecord(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	// Record first model.
	h.Record("openrouter", "model-a")
	if len(h.Recent) != 1 {
		t.Fatalf("expected 1 model, got %d", len(h.Recent))
	}
	if h.Recent[0] != "openrouter/model-a" {
		t.Fatalf("expected model-a, got %q", h.Recent[0])
	}

	// Record second model - should be at front.
	h.Record("anthropic", "claude-opus-4-7")
	if len(h.Recent) != 2 {
		t.Fatalf("expected 2 models, got %d", len(h.Recent))
	}
	if h.Recent[0] != "anthropic/claude-opus-4-7" {
		t.Fatalf("expected claude at front, got %q", h.Recent[0])
	}
}

func TestRecord_MovesExistingToFront(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	h.Record("openrouter", "model-a")
	h.Record("openrouter", "model-b")
	h.Record("openrouter", "model-c")

	// Record model-a again - should move to front.
	h.Record("openrouter", "model-a")

	if len(h.Recent) != 3 {
		t.Fatalf("expected 3 models, got %d", len(h.Recent))
	}
	if h.Recent[0] != "openrouter/model-a" {
		t.Fatalf("expected model-a at front, got %q", h.Recent[0])
	}
	// Order should be: model-a, model-c, model-b
	if h.Recent[1] != "openrouter/model-c" {
		t.Fatalf("expected model-c at index 1, got %q", h.Recent[1])
	}
	if h.Recent[2] != "openrouter/model-b" {
		t.Fatalf("expected model-b at index 2, got %q", h.Recent[2])
	}
}

func TestRecord_TrimsToMax(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	// Record MaxRecentModels + 1 models.
	for i := 0; i < MaxRecentModels+1; i++ {
		h.Record("openrouter", modelID(i))
	}

	if len(h.Recent) != MaxRecentModels {
		t.Fatalf("expected %d models, got %d", MaxRecentModels, len(h.Recent))
	}
	// The most recently recorded should be at front, the oldest trimmed.
	expectedFirst := "openrouter/" + modelID(MaxRecentModels)
	if h.Recent[0] != expectedFirst {
		t.Fatalf("expected %q at front, got %q", expectedFirst, h.Recent[0])
	}
}

func TestRecord_EmptyProviderOrModel(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	h.Record("", "model-a")
	if len(h.Recent) != 0 {
		t.Fatalf("expected no record for empty provider")
	}

	h.Record("openrouter", "")
	if len(h.Recent) != 0 {
		t.Fatalf("expected no record for empty model")
	}
}

func TestRecency(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	h.Record("openrouter", "model-a")
	h.Record("openrouter", "model-b")

	if idx := h.Recency("openrouter", "model-b"); idx != 0 {
		t.Fatalf("expected recency 0 for most recent, got %d", idx)
	}
	if idx := h.Recency("openrouter", "model-a"); idx != 1 {
		t.Fatalf("expected recency 1 for second, got %d", idx)
	}
	if idx := h.Recency("openrouter", "unknown"); idx != -1 {
		t.Fatalf("expected -1 for unknown model, got %d", idx)
	}
}

func TestRecency_Empty(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	if idx := h.Recency("", "model"); idx != -1 {
		t.Fatalf("expected -1 for empty provider")
	}
	if idx := h.Recency("provider", ""); idx != -1 {
		t.Fatalf("expected -1 for empty model")
	}
}

func TestPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)

	h.Record("openrouter", "model-a")
	h.Record("anthropic", "claude-opus-4-7")

	// Verify file exists.
	path := filepath.Join(dir, historyFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected history file at %s", path)
	}

	// Read file and verify content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty history file")
	}
}

func TestLoadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	h := NewModelHistory(dir)
	h.Record("openrouter", "model-a")

	// Create new instance - should load from disk.
	h2 := NewModelHistory(dir)
	if len(h2.Recent) != 1 {
		t.Fatalf("expected 1 model loaded from disk, got %d", len(h2.Recent))
	}
	if h2.Recent[0] != "openrouter/model-a" {
		t.Fatalf("expected model-a from disk, got %q", h2.Recent[0])
	}
}

func TestIgnoresCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, historyFileName)
	os.WriteFile(path, []byte("corrupted garbage\n"), 0o644)

	h := NewModelHistory(dir)
	if h == nil {
		t.Fatal("expected non-nil history from corrupted file")
	}
	if len(h.Recent) != 0 {
		t.Fatalf("expected empty history from corrupted file, got %d", len(h.Recent))
	}
}

func modelID(n int) string {
	return "model-" + fmt.Sprintf("%d", n)
}
