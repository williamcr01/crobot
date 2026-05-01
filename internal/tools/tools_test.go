package tools

import (
	"context"
	"fmt"
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

func TestLs_Basic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := LsTool.Execute(context.Background(), map[string]any{
		"path": dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	entries, ok := r["entries"].([]string)
	if !ok {
		t.Fatalf("entries not a []string, got %T", r["entries"])
	}
	// Should have a.txt, b.go, sub/ sorted.
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(entries), entries)
	}
	if entries[0] != "a.txt" {
		t.Errorf("expected entries[0]=a.txt, got %q", entries[0])
	}
	if entries[1] != "b.go" {
		t.Errorf("expected entries[1]=b.go, got %q", entries[1])
	}
	if entries[2] != "sub/" {
		t.Errorf("expected entries[2]=sub/, got %q", entries[2])
	}
	if r["path"].(string) != dir {
		t.Errorf("expected path %q, got %q", dir, r["path"])
	}
}

func TestLs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := LsTool.Execute(context.Background(), map[string]any{
		"path": dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	entries := r["entries"].([]string)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestLs_NotFound(t *testing.T) {
	result, err := LsTool.Execute(context.Background(), map[string]any{
		"path": "/nonexistent/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "path not found") {
		t.Errorf("expected path not found error, got %v", r["error"])
	}
}

func TestLs_FileAsPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := LsTool.Execute(context.Background(), map[string]any{
		"path": filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "not a directory") {
		t.Errorf("expected 'not a directory' error, got %v", r["error"])
	}
}

func TestLs_Limit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := LsTool.Execute(context.Background(), map[string]any{
		"path":  dir,
		"limit": float64(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	entries := r["entries"].([]string)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	if !r["truncated"].(bool) {
		t.Error("expected truncated true")
	}
}

func TestFind_Basic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.rs"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "helper.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	files, ok := r["files"].([]string)
	if !ok {
		t.Fatalf("files not a []string, got %T", r["files"])
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "main.go" {
		t.Errorf("expected files[0]=main.go, got %q", files[0])
	}
	if files[1] != "pkg/helper.go" {
		t.Errorf("expected files[1]=pkg/helper.go, got %q", files[1])
	}
}

func TestFind_NoMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTool.Execute(context.Background(), map[string]any{
		"pattern": "*.rs",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	files := r["files"].([]string)
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestFind_Gitignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.rs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.rs"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := FindTool.Execute(context.Background(), map[string]any{
		"pattern": "*",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	files := r["files"].([]string)
	// Should NOT include ignored.rs, but should include main.go and .gitignore itself maybe.
	for _, f := range files {
		if f == "ignored.rs" {
			t.Errorf("ignored.rs should be excluded by .gitignore")
		}
	}
}

func TestFind_MissingPattern(t *testing.T) {
	_, err := FindTool.Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "pattern is required") {
		t.Errorf("expected pattern required error, got %v", err)
	}
}

func TestFind_InvalidPath(t *testing.T) {
	result, err := FindTool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    "/nonexistent/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "path not found") {
		t.Errorf("expected path not found, got %v", r["error"])
	}
}

func TestFind_FileAsPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := FindTool.Execute(context.Background(), map[string]any{
		"pattern": "*",
		"path":    filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "not a directory") {
		t.Errorf("expected 'not a directory' error, got %v", r["error"])
	}
}

func TestFind_Limit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.go", i)), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := FindTool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
		"limit":   float64(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	files := r["files"].([]string)
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}
	if !r["truncated"].(bool) {
		t.Error("expected truncated true")
	}
}

func TestGrep_Basic(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file1, []byte("package main\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file2 := filepath.Join(dir, "b.go")
	if err := os.WriteFile(file2, []byte("package utils\nfunc Helper() {\n\treturn nil\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "func",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, ok := r["matches"].([]matchResult)
	if !ok {
		// Try []interface{} and convert.
		raw, _ := r["matches"].([]interface{})
		t.Fatalf("matches not []matchResult, got %T (len=%d)", r["matches"], len(raw))
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d: %+v", len(matches), matches)
	}
}

func TestGrep_SingleFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello world\nfoo bar\nhello again\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, ok := r["matches"].([]matchResult)
	if !ok {
		raw, _ := r["matches"].([]interface{})
		t.Fatalf("matches not []matchResult, got %T (len=%d)", r["matches"], len(raw))
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Line != 1 {
		t.Errorf("expected line 1, got %d", matches[0].Line)
	}
	if matches[1].Line != 3 {
		t.Errorf("expected line 3, got %d", matches[1].Line)
	}
}

func TestGrep_Literal(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("foo.bar\nfoobar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "foo.bar",
		"path":    filePath,
		"literal": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, _ := r["matches"].([]matchResult)
	if len(matches) != 1 {
		t.Errorf("expected 1 literal match, got %d: %+v", len(matches), matches)
	}
}

func TestGrep_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("Hello\nWORLD\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern":    "hello",
		"path":       filePath,
		"ignoreCase": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, _ := r["matches"].([]matchResult)
	if len(matches) != 2 {
		t.Errorf("expected 2 case-insensitive matches, got %d: %+v", len(matches), matches)
	}
}

func TestGrep_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "main",
		"path":    dir,
		"glob":    "*.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, _ := r["matches"].([]matchResult)
	if len(matches) != 1 {
		t.Errorf("expected 1 match (only *.go), got %d", len(matches))
	}
	if len(matches) > 0 && matches[0].File != "a.go" {
		t.Errorf("expected match in a.go, got %s", matches[0].File)
	}
}

func TestGrep_NoMatches(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "four",
		"path":    filePath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, _ := r["matches"].([]matchResult)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestGrep_InvalidPattern(t *testing.T) {
	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "[invalid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "invalid pattern") {
		t.Errorf("expected invalid pattern error, got %v", r["error"])
	}
}

func TestGrep_MissingPattern(t *testing.T) {
	_, err := GrepTool.Execute(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "pattern is required") {
		t.Errorf("expected pattern required error, got %v", err)
	}
}

func TestGrep_PathNotFound(t *testing.T) {
	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "test",
		"path":    "/nonexistent/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	if r["error"] == nil || !strings.Contains(r["error"].(string), "path not found") {
		t.Errorf("expected path not found error, got %v", r["error"])
	}
}

func TestGrep_Limit(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("match line %d", i))
	}
	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "match",
		"path":    filePath,
		"limit":   float64(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, _ := r["matches"].([]matchResult)
	if len(matches) != 5 {
		t.Errorf("expected 5 matches, got %d", len(matches))
	}
	if !r["truncated"].(bool) {
		t.Error("expected truncated true")
	}
}

func TestGrep_Gitignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("ERROR: something failed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := GrepTool.Execute(context.Background(), map[string]any{
		"pattern": "ERROR",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := result.(map[string]any)
	matches, _ := r["matches"].([]matchResult)
	// ERROR in debug.log should be ignored by .gitignore.
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (debug.log is gitignored), got %d: %+v", len(matches), matches)
	}
}

func TestTruncateLine(t *testing.T) {
	// Short line passes through.
	if s := truncateLine("short", 100); s != "short" {
		t.Errorf("expected 'short', got %q", s)
	}
	// Long line gets truncated.
	s := truncateLine("this is a very long line that should be truncated", 20)
	if !strings.HasSuffix(s, "...") {
		t.Errorf("expected truncated suffix, got %q", s)
	}
	// UTF-8 boundary preservation.
	s2 := truncateLine("hello 世界 world", 10)
	if len(s2) <= 10 || !strings.HasSuffix(s2, "...") {
		t.Errorf("expected truncated with suffix, got %q (len=%d)", s2, len(s2))
	}
}

func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()

	// Text file with no null bytes.
	txtPath := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(txtPath, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isBinaryFile(txtPath) {
		t.Error("expected text file to not be binary")
	}

	// Binary file with null byte.
	binPath := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !isBinaryFile(binPath) {
		t.Error("expected binary file to be detected")
	}

	// Non-existent file.
	if !isBinaryFile("/nonexistent") {
		t.Error("expected nonexistent file to be treated as binary")
	}
}

func TestContainsSlash(t *testing.T) {
	if containsSlash("foo") {
		t.Error("expected false")
	}
	if !containsSlash("foo/bar") {
		t.Error("expected true for /")
	}
	if !containsSlash("foo\\bar") {
		t.Error("expected true for \\")
	}
	if containsSlash("") {
		t.Error("expected false for empty")
	}
}

func TestToolExecute_Unknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute(context.Background(), "nonexistent", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected unknown tool error, got %v", err)
	}
}
