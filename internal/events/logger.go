package events

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType identifies the kind of event.
type EventType string

const (
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventAPIRequest EventType = "api_request"
	EventAPIResponse EventType = "api_response"
	EventError      EventType = "error"
	EventPluginLoad EventType = "plugin_load"
)

// Event is a structured log entry.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

// Logger writes structured events as JSON lines.
type Logger struct {
	w  io.Writer
	mu sync.Mutex
}

// NewLogger creates an event logger.
// If sessionDir is non-empty, it writes to events-{date}.jsonl in that directory.
// Otherwise, it writes to stderr.
func NewLogger(sessionDir string) *Logger {
	if sessionDir == "" {
		return &Logger{w: os.Stderr}
	}

	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return &Logger{w: os.Stderr}
	}

	date := time.Now().Format("2006-01-02")
	path := filepath.Join(sessionDir, fmt.Sprintf("events-%s.jsonl", date))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return &Logger{w: os.Stderr}
	}

	return &Logger{w: f}
}

func (l *Logger) log(typ EventType, data map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ev := Event{
		Type:      typ,
		Timestamp: time.Now(),
		Data:      data,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = l.w.Write(line)
	_, _ = l.w.Write([]byte("\n"))
}

// ToolCall logs a tool call event.
func (l *Logger) ToolCall(name string, args map[string]any) {
	l.log(EventToolCall, map[string]any{
		"name": name,
		"args": args,
	})
}

// ToolResult logs a tool result event.
func (l *Logger) ToolResult(name string, durationMs int64, outputSize int, err error) {
	data := map[string]any{
		"name":        name,
		"duration_ms": durationMs,
		"output_size": outputSize,
	}
	if err != nil {
		data["error"] = err.Error()
	}
	l.log(EventToolResult, data)
}

// APIRequest logs an API request event.
func (l *Logger) APIRequest(model string, messageCount int) {
	l.log(EventAPIRequest, map[string]any{
		"model":         model,
		"message_count": messageCount,
	})
}

// APIResponse logs an API response event.
func (l *Logger) APIResponse(model string, inputTokens, outputTokens int, durationMs int64) {
	l.log(EventAPIResponse, map[string]any{
		"model":          model,
		"input_tokens":   inputTokens,
		"output_tokens":  outputTokens,
		"duration_ms":    durationMs,
	})
}

// Error logs an error event.
func (l *Logger) Error(context string, err error) {
	l.log(EventError, map[string]any{
		"context": context,
		"message": err.Error(),
	})
}

// PluginLoad logs a plugin lifecycle event.
func (l *Logger) PluginLoad(name, version, status string) {
	l.log(EventPluginLoad, map[string]any{
		"name":    name,
		"version": version,
		"status":  status,
	})
}
