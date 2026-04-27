package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLogger_OutputBuffer(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{w: &buf, mu: sync.Mutex{}}

	l.ToolCall("file_read", map[string]any{"path": "/tmp/test.txt"})
	l.ToolResult("file_read", 150, 1024, nil)
	l.APIRequest("gpt-4", 5)
	l.APIResponse("gpt-4", 100, 50, 1200)
	l.Error("runner", errors.New("connection failed"))
	l.PluginLoad("echo", "1.0.0", "loaded")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 event lines, got %d", len(lines))
	}

	// Each line must be valid JSON.
	for i, line := range lines {
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("line %d missing timestamp", i)
		}
	}
}

func TestLogger_EventTypes(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{w: &buf, mu: sync.Mutex{}}

	l.ToolCall("shell", map[string]any{"command": "ls"})
	l.Error("parser", errors.New("syntax error"))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	var ev1 Event
	json.Unmarshal([]byte(lines[0]), &ev1)
	if ev1.Type != EventToolCall {
		t.Errorf("expected tool_call, got %s", ev1.Type)
	}
	if ev1.Data["name"] != "shell" {
		t.Errorf("expected name shell, got %v", ev1.Data["name"])
	}

	var ev2 Event
	json.Unmarshal([]byte(lines[1]), &ev2)
	if ev2.Type != EventError {
		t.Errorf("expected error, got %s", ev2.Type)
	}
	if ev2.Data["message"] != "syntax error" {
		t.Errorf("expected syntax error, got %v", ev2.Data["message"])
	}
}

func TestLogger_FileOutput(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir)

	l.ToolCall("file_write", map[string]any{"path": "/tmp/out.txt", "written": 42})

	// Check a file was created in the dir.
	entries, err := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 events file, got %d: %v", len(entries), entries)
	}

	// Read it back.
	var ev Event
	b, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != EventToolCall {
		t.Errorf("expected tool_call, got %s", ev.Type)
	}
}

func TestLogger_EmptyDirStderr(t *testing.T) {
	l := NewLogger("")
	if l.w == nil {
		t.Fatal("logger writer should not be nil")
	}
	// Should not panic.
	l.ToolCall("test", nil)
}

func TestLogger_ToolResultError(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{w: &buf, mu: sync.Mutex{}}

	l.ToolResult("shell", 200, 0, errors.New("exit code 1"))

	var ev Event
	json.Unmarshal(buf.Bytes(), &ev)
	if ev.Type != EventToolResult {
		t.Errorf("expected tool_result, got %s", ev.Type)
	}
	if ev.Data["error"] != "exit code 1" {
		t.Errorf("expected error message, got %v", ev.Data["error"])
	}
}

func TestLogger_Concurrency(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{w: &buf, mu: sync.Mutex{}}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.ToolCall("concurrent_test", nil)
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 20 {
		t.Errorf("expected 20 lines, got %d", len(lines))
	}
}
