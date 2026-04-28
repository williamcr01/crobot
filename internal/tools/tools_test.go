package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileRead_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileReadTool.Execute(context.Background(), map[string]any{
		"path": path,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["content"].(string) != "line1\nline2\nline3\nline4\nline5\n" {
		t.Errorf("unexpected content: %q", r["content"])
	}
	if r["totalLines"].(int) != 6 {
		t.Errorf("expected 6 lines, got %d", r["totalLines"])
	}
}

func TestFileRead_WithOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileReadTool.Execute(context.Background(), map[string]any{
		"path":   path,
		"offset": float64(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["content"].(string) != "line3\nline4\nline5\n" {
		t.Errorf("unexpected content: %q", r["content"])
	}
}

func TestFileRead_WithLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileReadTool.Execute(context.Background(), map[string]any{
		"path":  path,
		"limit": float64(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["content"].(string) != "line1\nline2" {
		t.Errorf("unexpected content: %q", r["content"])
	}
	if !r["truncated"].(bool) {
		t.Error("expected truncated true")
	}
	if r["nextOffset"].(int) != 3 {
		t.Errorf("expected nextOffset 3, got %d", r["nextOffset"])
	}
}

func TestFileRead_WithOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileReadTool.Execute(context.Background(), map[string]any{
		"path":   path,
		"offset": float64(2),
		"limit":  float64(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["content"].(string) != "line2\nline3" {
		t.Errorf("unexpected content: %q", r["content"])
	}
}

func TestFileRead_NotFound(t *testing.T) {
	result, err := FileReadTool.Execute(context.Background(), map[string]any{
		"path": "/nonexistent/path/file.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "File not found") {
		t.Errorf("expected file not found error, got %v", r["error"])
	}
}

func TestFileRead_MissingPath(t *testing.T) {
	_, err := FileReadTool.Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Errorf("expected path required error, got %v", err)
	}
}

func TestFileRead_ImageDetection(t *testing.T) {
	// Test isImage and detectMIME.
	if !isImage([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}) {
		t.Error("expected PNG detected")
	}
	if !isImage([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Error("expected JPEG detected")
	}
	if !isImage([]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0, 0, 0, 0, 0, 0}) {
		t.Error("expected GIF detected")
	}
	// WebP: RIFF + WEBP
	webp := []byte{0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50}
	if !isImage(webp) {
		t.Error("expected WebP detected")
	}
	if isImage([]byte("not an image")) {
		t.Error("expected not an image")
	}
	if isImage([]byte{0, 0, 0, 0}) {
		t.Error("expected not an image (too short)")
	}

	// detectMIME checks len(data) < 12, so we need 12+ bytes.
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}
	gifHeader := []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0, 0, 0, 0, 0, 0}
	webpHeader := []byte{0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50}

	if m := detectMIME(pngHeader); m != "image/png" {
		t.Errorf("expected image/png, got %s", m)
	}
	if m := detectMIME(jpegHeader); m != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", m)
	}
	if m := detectMIME(gifHeader); m != "image/gif" {
		t.Errorf("expected image/gif, got %s", m)
	}
	if m := detectMIME(webpHeader); m != "image/webp" {
		t.Errorf("expected image/webp, got %s", m)
	}
	if m := detectMIME([]byte("hello")); m != "application/octet-stream" {
		t.Errorf("expected application/octet-stream, got %s", m)
	}
	if m := detectMIME(nil); m != "application/octet-stream" {
		t.Errorf("expected application/octet-stream for nil, got %s", m)
	}
}

func TestFileWrite_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.txt")

	result, err := FileWriteTool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if !r["success"].(bool) {
		t.Error("expected success")
	}
	if r["written"].(int) != 11 {
		t.Errorf("expected 11 bytes, got %d", r["written"])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestFileWrite_MissingPath(t *testing.T) {
	_, err := FileWriteTool.Execute(context.Background(), map[string]any{
		"content": "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Errorf("expected path required error, got %v", err)
	}
}

func TestFileEdit_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("foo bar baz"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileEditTool.Execute(context.Background(), map[string]any{
		"path":    path,
		"oldText": "bar",
		"newText": "qux",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if !r["success"].(bool) {
		t.Error("expected success")
	}
	if r["line"].(int) != 1 {
		t.Errorf("expected line 1, got %d", r["line"])
	}

	data, _ := os.ReadFile(path)
	if string(data) != "foo qux baz" {
		t.Errorf("expected 'foo qux baz', got %q", string(data))
	}
}

func TestFileEdit_MultiLineLineNumber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\nline4"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileEditTool.Execute(context.Background(), map[string]any{
		"path":    path,
		"oldText": "line3",
		"newText": "CHANGED",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["line"].(int) != 3 {
		t.Errorf("expected line 3, got %d", r["line"])
	}
}

func TestFileEdit_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("foo bar baz"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileEditTool.Execute(context.Background(), map[string]any{
		"path":    path,
		"oldText": "nonexistent",
		"newText": "replacement",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "not found") {
		t.Errorf("expected not found error, got %v", r["error"])
	}
}

func TestFileEdit_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("foo bar foo"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FileEditTool.Execute(context.Background(), map[string]any{
		"path":    path,
		"oldText": "foo",
		"newText": "qux",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "matches 2") {
		t.Errorf("expected matches error, got %v", r["error"])
	}
}

func TestFileEdit_MissingPath(t *testing.T) {
	_, err := FileEditTool.Execute(context.Background(), map[string]any{
		"oldText": "foo",
		"newText": "bar",
	})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Errorf("expected path required error, got %v", err)
	}
}

func TestFileEdit_FileNotFound(t *testing.T) {
	result, err := FileEditTool.Execute(context.Background(), map[string]any{
		"path":    "/nonexistent/file.txt",
		"oldText": "foo",
		"newText": "bar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "File not found") {
		t.Errorf("expected File not found error, got %v", r["error"])
	}
}

func TestShell_Basic(t *testing.T) {
	result, err := BashTool.Execute(context.Background(), map[string]any{
		"command": "echo hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["stdout"].(string) != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", r["stdout"])
	}
	if r["exitCode"].(int) != 0 {
		t.Errorf("expected exit code 0, got %d", r["exitCode"])
	}
}

func TestShell_NonZeroExit(t *testing.T) {
	result, err := BashTool.Execute(context.Background(), map[string]any{
		"command": "exit 42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["exitCode"].(int) != 42 {
		t.Errorf("expected exit code 42, got %d", r["exitCode"])
	}
}

func TestShell_Stderr(t *testing.T) {
	result, err := BashTool.Execute(context.Background(), map[string]any{
		"command": "echo error >&2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["stderr"].(string) != "error\n" {
		t.Errorf("expected 'error\\n', got %q", r["stderr"])
	}
}

func TestShell_MissingCommand(t *testing.T) {
	_, err := BashTool.Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Errorf("expected command required error, got %v", err)
	}
}

func TestShell_Timeout(t *testing.T) {
	result, err := BashTool.Execute(context.Background(), map[string]any{
		"command": "sleep 0.05 && echo done",
		"timeout": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["stdout"].(string) != "done\n" {
		t.Errorf("expected 'done\\n', got %q", r["stdout"])
	}
	if r["exitCode"].(int) != 0 {
		t.Errorf("expected exit code 0, got %d", r["exitCode"])
	}
}

func TestShell_TimeoutClamps(t *testing.T) {
	// timeout > 120 should clamp to 120 but the test doesn't need to verify that
	// precisely; we just ensure it doesn't blow up.
	result, err := BashTool.Execute(context.Background(), map[string]any{
		"command": "echo ok",
		"timeout": float64(999),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["stdout"].(string) != "ok\n" {
		t.Errorf("expected 'ok\\n', got %q", r["stdout"])
	}
}

func TestTruncateOutput(t *testing.T) {
	if s := truncateOutput("short", 100); s != "short" {
		t.Errorf("expected 'short', got %q", s)
	}
	// Truncation appends suffix.
	s := truncateOutput("hello world", 5)
	if !strings.HasSuffix(s, "... (truncated)") {
		t.Errorf("expected truncated suffix, got %q", s)
	}
	if len(s) < len("... (truncated)") {
		t.Errorf("result too short: %q", s)
	}
	// UTF-8 boundary preservation.
	s2 := truncateOutput("hello 世界 world", 10)
	if !strings.HasSuffix(s2, "... (truncated)") {
		t.Errorf("expected truncated suffix, got %q", s2)
	}
	// String under limit passes through.
	s3 := truncateOutput("hi", 5)
	if s3 != "hi" {
		t.Errorf("expected 'hi', got %q", s3)
	}
}

func TestToolGetProviderTools(t *testing.T) {
	reg := NewRegistry()
	reg.Register(FileReadTool)
	reg.Register(FileWriteTool)

	tools := reg.ToProviderTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
	// Verify both tools are present.
	names := map[string]bool{}
	for _, tt := range tools {
		names[tt.Name] = true
	}
	if !names["file_read"] {
		t.Error("missing file_read")
	}
	if !names["file_write"] {
		t.Error("missing file_write")
	}
}

func TestToolExecute_Unknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute(context.Background(), "nonexistent", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected unknown tool error, got %v", err)
	}
}
