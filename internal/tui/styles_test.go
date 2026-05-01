package tui

import (
	"strings"
	"testing"
	"time"
)

// testStyles is declared in markdown_test.go

func TestRenderToolCall_Pending(t *testing.T) {
	tc := toolRenderItem{
		name:   "bash",
		callID: "call_1",
		args:   "echo hello",
		state:  toolPending,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "echo hello") {
		t.Fatalf("expected bash command in output: %q", stripped)
	}
}

func TestRenderToolCall_Running(t *testing.T) {
	tc := toolRenderItem{
		name:   "read",
		callID: "call_2",
		args:   "file.go",
		state:  toolRunning,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "running") {
		t.Fatalf("expected running status: %q", stripped)
	}
}

func TestRenderToolCall_DoneSuccess(t *testing.T) {
	tc := toolRenderItem{
		name:     "bash",
		callID:   "call_3",
		args:     "echo hello",
		output:   "hello\n",
		success:  true,
		duration: 1500 * time.Millisecond,
		state:    toolDone,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "hello") {
		t.Fatalf("expected output in tool call: %q", stripped)
	}
	if !strings.Contains(stripped, "ok") {
		t.Fatalf("expected ok status: %q", stripped)
	}
}

func TestRenderToolCall_DoneError(t *testing.T) {
	tc := toolRenderItem{
		name:     "bash",
		callID:   "call_4",
		args:     "false",
		output:   "exit code 1",
		success:  false,
		duration: 500 * time.Millisecond,
		state:    toolDone,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "err") {
		t.Fatalf("expected err status: %q", stripped)
	}
}

func TestRenderToolCall_ExpandedOutput(t *testing.T) {
	outputLines := make([]string, 20)
	for i := 0; i < 20; i++ {
		outputLines[i] = "line"
	}
	tc := toolRenderItem{
		name:     "bash",
		callID:   "call_5",
		args:     "echo stuff",
		output:   strings.Join(outputLines, "\n"),
		success:  true,
		duration: 1 * time.Second,
		state:    toolDone,
	}

	// Collapsed: should show preview with "... N more lines"
	collapsed := RenderToolCall(tc, 80, false, testStyles)
	strippedCollapsed := stripANSI(collapsed)
	if !strings.Contains(strippedCollapsed, "more lines") {
		t.Fatalf("expected collapsed preview hint: %q", strippedCollapsed)
	}

	// Expanded: should show all lines without hint
	expanded := RenderToolCall(tc, 80, true, testStyles)
	strippedExpanded := stripANSI(expanded)
	if strings.Contains(strippedExpanded, "more lines") {
		t.Fatalf("expected expanded output without hint: %q", strippedExpanded)
	}
}

func TestRenderToolCall_NoOutput(t *testing.T) {
	tc := toolRenderItem{
		name:    "read",
		callID:  "call_6",
		args:    "empty.txt",
		output:  "",
		success: true,
		state:   toolDone,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if strings.Contains(stripped, "more lines") {
		t.Fatalf("expected no preview hint for empty output: %q", stripped)
	}
}

func TestRenderToolCall_NarrowWidth(t *testing.T) {
	tc := toolRenderItem{
		name:   "bash",
		callID: "call_7",
		args:   "very long command that should be truncated",
		state:  toolPending,
	}
	got := RenderToolCall(tc, 30, false, testStyles)
	stripped := stripANSI(got)
	// Should not panic on narrow width.
	if !strings.Contains(stripped, "very") {
		t.Fatalf("expected partial command in narrow render: %q", stripped)
	}
}

func TestFormatSingleToolCallLine_Bash(t *testing.T) {
	tc := toolRenderItem{
		name: "bash",
		args: "ls -la",
	}
	got := formatSingleToolCallLine(tc, testStyles)
	if !strings.Contains(stripANSI(got), "ls -la") {
		t.Fatalf("expected bash command in line: %q", got)
	}
}

func TestFormatSingleToolCallLine_NonBash(t *testing.T) {
	tc := toolRenderItem{
		name: "read",
		args: "file.go",
	}
	got := formatSingleToolCallLine(tc, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "read") {
		t.Fatalf("expected tool name: %q", stripped)
	}
	if !strings.Contains(stripped, "file.go") {
		t.Fatalf("expected tool args: %q", stripped)
	}
}

func TestFormatSingleToolCallLine_NoArgs(t *testing.T) {
	tc := toolRenderItem{
		name: "read",
		args: "",
	}
	got := formatSingleToolCallLine(tc, testStyles)
	stripped := stripANSI(got)
	if stripped != "read" {
		t.Fatalf("expected just tool name, got: %q", stripped)
	}
}

func TestFormatOutputPreview_Capped(t *testing.T) {
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, "some output line")
	}
	output := strings.Join(lines, "\n")
	got := formatOutputPreview(output, 80, 5, true, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "10 more lines") {
		t.Fatalf("expected preview cap hint: %q", stripped)
	}
}

func TestFormatOutputPreview_Uncapped(t *testing.T) {
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, "some output line")
	}
	output := strings.Join(lines, "\n")
	got := formatOutputPreview(output, 80, 20, false, testStyles)
	stripped := stripANSI(got)
	if strings.Contains(stripped, "more lines") {
		t.Fatalf("expected no cap hint when maxLines > len(lines): %q", stripped)
	}
}

func TestFormatOutputPreview_EmptyOutput(t *testing.T) {
	got := formatOutputPreview("", 80, 5, true, testStyles)
	if got != "" {
		t.Fatalf("expected empty for empty output, got: %q", got)
	}
}

func TestFormatDuration_Zero(t *testing.T) {
	if got := formatDuration(0); got != "0ms" {
		t.Fatalf("expected 0ms, got %q", got)
	}
}

func TestFormatDuration_Milliseconds(t *testing.T) {
	if got := formatDuration(450 * time.Millisecond); got != "450ms" {
		t.Fatalf("expected 450ms, got %q", got)
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	if got := formatDuration(2300 * time.Millisecond); got != "2.3s" {
		t.Fatalf("expected 2.3s, got %q", got)
	}
}

func TestFormatDuration_ExactSecond(t *testing.T) {
	if got := formatDuration(3000 * time.Millisecond); got != "3.0s" {
		t.Fatalf("expected 3.0s, got %q", got)
	}
}

func TestStatusLabel_Success(t *testing.T) {
	if got := statusLabel(true); got != "ok" {
		t.Fatalf("expected ok, got %q", got)
	}
}

func TestStatusLabel_Error(t *testing.T) {
	if got := statusLabel(false); got != "err" {
		t.Fatalf("expected err, got %q", got)
	}
}

func TestTruncateToWidth_ShortString(t *testing.T) {
	if got := truncateToWidth("hello", 10); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestTruncateToWidth_ExactFit(t *testing.T) {
	if got := truncateToWidth("hello", 5); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestTruncateToWidth_TooLong(t *testing.T) {
	got := truncateToWidth("hello world", 8)
	// maxWidth-1 = 7 visible runes + ellipsis = 8 total
	if got != "hello w…" {
		t.Fatalf("expected 'hello w…', got %q (len=%d)", got, len([]rune(got)))
	}
}

func TestTruncateToWidth_Boundary(t *testing.T) {
	got := truncateToWidth("hello", 1)
	if got != "…" {
		t.Fatalf("expected '…', got %q", got)
	}

	got2 := truncateToWidth("ab", 2)
	if got2 != "ab" {
		t.Fatalf("expected 'ab', got %q", got2)
	}
}

func TestTruncateToWidth_ZeroWidth(t *testing.T) {
	if got := truncateToWidth("hello", 0); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestTruncateToWidth_EmptyString(t *testing.T) {
	if got := truncateToWidth("", 10); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestRenderToolCall_WidthBelowMinimum(t *testing.T) {
	tc := toolRenderItem{
		name:   "bash",
		callID: "call_1",
		args:   "echo test",
		state:  toolPending,
	}
	got := RenderToolCall(tc, 10, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "echo test") {
		t.Fatalf("expected command in narrow render: %q", stripped)
	}
}

func TestRenderToolCall_OutputWithTrailingEmptyLines(t *testing.T) {
	tc := toolRenderItem{
		name:     "bash",
		callID:   "call_trail",
		args:     "echo test",
		output:   "result\n\n\n",
		success:  true,
		duration: 100 * time.Millisecond,
		state:    toolDone,
	}
	got := RenderToolCall(tc, 80, true, testStyles)
	stripped := stripANSI(got)
	if strings.Contains(stripped, "\n\n\n") {
		t.Fatalf("expected trailing empty lines to be stripped: %q", stripped)
	}
}

func TestRenderToolCall_HiddenCountOverflow(t *testing.T) {
	outputLines := make([]string, 15)
	for i := 0; i < 15; i++ {
		outputLines[i] = "data"
	}
	tc := toolRenderItem{
		name:     "bash",
		callID:   "call_hidden",
		args:     "echo data",
		output:   strings.Join(outputLines, "\n"),
		success:  true,
		duration: 1 * time.Second,
		state:    toolDone,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "5 more lines") {
		t.Fatalf("expected '5 more lines' in collapsed output: %q", stripped)
	}
}

func TestFormatOutputPreview_NotCollapsedHiddenCount(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "data")
	}
	output := strings.Join(lines, "\n")
	got := formatOutputPreview(output, 80, 5, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "15 more lines") {
		t.Fatalf("expected hidden count in expanded form: %q", stripped)
	}
}

func TestRenderToolCall_StatusLineWithEmptyOutput(t *testing.T) {
	tc := toolRenderItem{
		name:     "bash",
		callID:   "call_status",
		args:     "echo done",
		output:   "",
		success:  true,
		duration: 50 * time.Millisecond,
		state:    toolDone,
	}
	got := RenderToolCall(tc, 80, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "50ms") {
		t.Fatalf("expected duration in status line: %q", stripped)
	}
}

func TestFmtSprintf(t *testing.T) {
	got := fmtSprintf("hello %s", "world")
	if got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}
}

func TestRenderToolCall_BashHeaderStyle(t *testing.T) {
	tc := toolRenderItem{
		name: "bash",
		args: "curl https://example.com",
	}
	line := formatSingleToolCallLine(tc, testStyles)
	// Just verify the command text appears
	if !strings.Contains(stripANSI(line), "curl https://example.com") {
		t.Fatalf("expected bash command in styled line: %q", line)
	}
}
