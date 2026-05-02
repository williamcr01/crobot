package tui

import (
	"strings"
	"testing"

	"crobot/internal/config"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestStripANSI_RemovesAllEscapeSequences(t *testing.T) {
	styled := "\x1b[1mBold\x1b[0m text \x1b[34mblue\x1b[0m"
	got := stripANSI(styled)
	want := "Bold text blue"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestStripANSI_EmptyString(t *testing.T) {
	if got := stripANSI(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestStripANSI_NoEscapeSequences(t *testing.T) {
	plain := "hello world"
	if got := stripANSI(plain); got != plain {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestStyledColToPlainOffset_Basic(t *testing.T) {
	styled := "\x1b[1mHello\x1b[0m world"
	// Characters: H e l l o (5 visible) then space w o r l d (6 more = 11 total)
	// Plain: "Hello world" (11 chars)
	tests := []struct {
		styledCol int
		want      int
	}{
		{0, 0},
		{1, 1},
		{4, 4},
		{5, 5}, // position 5 in plain = " " (space after Hello)
		{6, 6}, // 'w'
		{10, 10},
		{11, 11}, // at end
		{100, 11},
	}
	for _, tc := range tests {
		got := styledColToPlainOffset(styled, tc.styledCol)
		if got != tc.want {
			t.Errorf("styledColToPlainOffset(%q, %d) = %d, want %d", styled, tc.styledCol, got, tc.want)
		}
	}
}

func TestStyledColToPlainOffset_EmptyString(t *testing.T) {
	got := styledColToPlainOffset("", 5)
	if got != 0 {
		t.Fatalf("expected 0 for empty string, got %d", got)
	}
}

func TestPlainOffsetToStyledPos_Basic(t *testing.T) {
	styled := "\x1b[1mHello\x1b[0m world"
	// Plain: "Hello world" (11 chars)
	// Styled positions: ANSI len = 4+4 = 8 bytes of escape codes before "Hello" + "Hello" = 5 chars, total 13 for 'H' at pos 0 plain.
	// Actually: ESC[1m = 4 bytes, then H=pos4, e=5, l=6, l=7, o=8, ESC[0m = 4 bytes (pos 9-12), ' ' = pos 13, w=14...
	tests := []struct {
		plainOffset int
		want        int
	}{
		{0, 4},    // 'H' starts after \x1b[1m (4 bytes)
		{1, 5},    // 'e'
		{4, 8},    // 'o'
		{5, 13},   // ' ' (after \x1b[0m at 9-12)
		{6, 14},   // 'w'
		{11, 19},  // end of plain text = len(styled)
		{100, 19}, // beyond end = len(styled)
	}
	for _, tc := range tests {
		got := plainOffsetToStyledPos(styled, tc.plainOffset)
		if got != tc.want {
			t.Errorf("plainOffsetToStyledPos(%q, %d) = %d, want %d", styled, tc.plainOffset, got, tc.want)
		}
	}
}

func TestPlainOffsetToStyledPos_EmptyString(t *testing.T) {
	got := plainOffsetToStyledPos("", 5)
	if got != 0 {
		t.Fatalf("expected 0 for empty string, got %d", got)
	}
}

func TestPlainOffsetToStyledPos_NoEscapeSequences(t *testing.T) {
	plain := "hello"
	for i := 0; i <= len(plain); i++ {
		got := plainOffsetToStyledPos(plain, i)
		if got != i {
			t.Errorf("plainOffsetToStyledPos(%q, %d) = %d, want %d", plain, i, got, i)
		}
	}
}

func TestSelectionState_Normalize_Unordered(t *testing.T) {
	s := selectionState{
		startLine: 5, startCol: 10,
		endLine: 3, endCol: 5,
	}
	s.normalize()
	if s.startLine != 3 || s.startCol != 5 || s.endLine != 5 || s.endCol != 10 {
		t.Fatalf("unexpected normalized: start=(%d,%d) end=(%d,%d)", s.startLine, s.startCol, s.endLine, s.endCol)
	}
}

func TestSelectionState_Normalize_SameLine(t *testing.T) {
	s := selectionState{
		startLine: 2, startCol: 15,
		endLine: 2, endCol: 3,
	}
	s.normalize()
	if s.startCol != 3 || s.endCol != 15 {
		t.Fatalf("same-line normalize: startCol=%d endCol=%d", s.startCol, s.endCol)
	}
}

func TestSelectionState_HasSelection(t *testing.T) {
	sel := selectionState{startLine: 0, startCol: 0, endLine: 0, endCol: 5}
	if !sel.hasSelection() {
		t.Fatal("expected hasSelection=true for valid selection")
	}

	sel2 := selectionState{startLine: 0, startCol: 3, endLine: 0, endCol: 3}
	if sel2.hasSelection() {
		t.Fatal("expected hasSelection=false for zero-width selection")
	}

	sel3 := selectionState{startLine: -1, startCol: 0, endLine: -1, endCol: 0}
	if sel3.hasSelection() {
		t.Fatal("expected hasSelection=false for negative lines")
	}
}

func TestSelectionState_SelectedText_SingleLine(t *testing.T) {
	sel := selectionState{
		startLine: 1, startCol: 6,
		endLine: 1, endCol: 11,
	}
	plainLines := []string{"line 0", "Hello World here", "line 2"}
	got := sel.selectedText(plainLines)
	want := "World"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSelectionState_SelectedText_MultiLine(t *testing.T) {
	sel := selectionState{
		startLine: 1, startCol: 3,
		endLine: 3, endCol: 5,
	}
	plainLines := []string{"line0", "abcdef", "ghijkl", "mnopqr", "line4"}
	got := sel.selectedText(plainLines)
	want := "def\nghijkl\nmnopq"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSelectionState_SelectedText_BoundaryStart(t *testing.T) {
	sel := selectionState{
		startLine: 0, startCol: 0,
		endLine: 0, endCol: 5,
	}
	plainLines := []string{"Hello World"}
	got := sel.selectedText(plainLines)
	want := "Hello"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSelectionState_SelectedText_BoundaryEnd(t *testing.T) {
	sel := selectionState{
		startLine: 0, startCol: 6,
		endLine: 0, endCol: 11,
	}
	plainLines := []string{"Hello World"}
	got := sel.selectedText(plainLines)
	want := "World"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSelectionState_SelectedText_OutOfBounds(t *testing.T) {
	sel := selectionState{
		startLine: 10, startCol: 0,
		endLine: 20, endCol: 10,
	}
	plainLines := []string{"only one line"}
	got := sel.selectedText(plainLines)
	if got != "" {
		t.Fatalf("expected empty for out-of-bounds, got %q", got)
	}
}

func TestOverlaySelection_SingleLine(t *testing.T) {
	content := "plain line\nnot selected\nanother line"
	sel := selectionState{
		startLine: 1, startCol: 4,
		endLine: 1, endCol: 12,
	}
	got := overlaySelection(content, sel)
	lines := strings.Split(got, "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Line 0 should be untouched.
	if lines[0] != "plain line" {
		t.Fatalf("line 0 changed: %q", lines[0])
	}

	// Line 1 should have reverse video around " selecte".
	if !strings.Contains(lines[1], "\x1b[7m") || !strings.Contains(lines[1], "\x1b[27m") {
		t.Fatalf("line 1 missing reverse video: %q", lines[1])
	}

	// Line 2 should be untouched.
	if lines[2] != "another line" {
		t.Fatalf("line 2 changed: %q", lines[2])
	}
}

func TestOverlaySelection_NoSelection(t *testing.T) {
	content := "hello\nworld"
	sel := selectionState{startLine: -1, endLine: -1}
	got := overlaySelection(content, sel)
	if got != content {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestStyleLineForSelection_Partial(t *testing.T) {
	styled := "Hello World"
	got := styleLineForSelection(styled, 0, 5)
	want := "\x1b[7mHello\x1b[27m World"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSelectionState_Clear(t *testing.T) {
	sel := selectionState{
		active: true, finished: true,
		startLine: 3, startCol: 5,
		endLine: 7, endCol: 10,
	}
	sel.clear()
	if sel.active || sel.finished || sel.startLine >= 0 || sel.endLine >= 0 {
		t.Fatalf("clear did not reset state: %+v", sel)
	}
}

func TestHandleMouseSelection_ViewportBoundary(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 15) // viewport height = 15
	m.viewport.SetContent(strings.Repeat("x\n", 30))
	m.refreshViewport()

	// Click at Y=10, which is within the viewport (10 < 15).
	msg := tea.MouseMsg{
		X:      5,
		Y:      10,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	consumed := m.handleMouseSelection(msg)
	if !consumed {
		t.Fatal("expected click inside viewport to be consumed")
	}
	if !m.selection.active {
		t.Fatal("expected selection to be active after press inside viewport")
	}

	// Clear and try a click at Y=20, which is outside the viewport (20 >= 15).
	m.selection.clear()
	msg.Y = 20
	consumed = m.handleMouseSelection(msg)
	if consumed {
		t.Fatal("expected click outside viewport to not be consumed")
	}
	if m.selection.active {
		t.Fatal("expected selection not active after press outside viewport")
	}
}

func TestHandleMouseSelection_DynamicViewportHeight(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	// Viewport stored height is 15, but dynamic height might differ.
	m.viewport = viewport.New(80, 15)
	m.viewport.SetContent(strings.Repeat("x\n", 30))
	m.refreshViewport()

	// With pending=true, dynamicViewportHeight adjusts (footer grows).
	m.pending = true

	dynamicHeight := m.dynamicViewportHeight()
	t.Logf("dynamic viewport height = %d, viewport.Height = %d", dynamicHeight, m.viewport.Height)

	// Click at Y=17 — this would be outside m.viewport.Height (15) but inside dynamic (18).
	msg := tea.MouseMsg{
		X:      5,
		Y:      17,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	consumed := m.handleMouseSelection(msg)
	if !consumed {
		t.Fatalf("expected click at Y=17 to be consumed when dynamic height is %d (stored height: %d)", dynamicHeight, m.viewport.Height)
	}
}

func TestHandleMouseSelection_OutsideClearsExistingSelection(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 15)
	m.viewport.SetContent(strings.Repeat("x\n", 30))
	m.refreshViewport()

	// Set an existing selection.
	m.selection.finished = true
	m.selection.startLine = 3
	m.selection.startCol = 5
	m.selection.endLine = 7
	m.selection.endCol = 10

	// Click outside viewport.
	msg := tea.MouseMsg{
		X:      5,
		Y:      20,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	consumed := m.handleMouseSelection(msg)
	if !consumed {
		t.Fatal("expected click outside to clear selection (consumed)")
	}
	if m.selection.hasSelection() || m.selection.finished {
		t.Fatal("expected selection to be cleared after click outside")
	}
}

// TestUpdateMouseSelection verifies the full Update flow for mouse selection.
func TestUpdateMouseSelection_FullFlow(t *testing.T) {
	m := NewModel(&config.AgentConfig{HasAuthorizedProvider: true}, nil, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24

	// Add real messages so renderMessages produces meaningful, tall content.
	for i := 0; i < 30; i++ {
		m.messages = append(m.messages, messageItem{
			role:    "system",
			content: strings.Repeat("abcdefghij", 6), // 60 chars per line
		})
	}
	// Force viewport init.
	m.viewport = viewport.New(80, 20)
	m.refreshViewport()

	// Step 1: Mouse press — start selection.
	pressMsg := tea.MouseMsg{
		X:      3,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	updated, cmd := m.Update(pressMsg)
	m2 := updated.(Model)
	if cmd != nil {
		t.Fatalf("unexpected cmd after press: %v", cmd)
	}
	if !m2.selection.active {
		t.Fatal("expected selection.active = true after mouse press")
	}

	// Step 2: Mouse motion — extend selection.
	motionMsg := tea.MouseMsg{
		X:      8,
		Y:      7,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	}
	updated, cmd = m2.Update(motionMsg)
	m3 := updated.(Model)
	if !m3.selection.active {
		t.Fatal("expected selection.active to remain true during drag")
	}

	// Step 3: Mouse release — finish selection.
	releaseMsg := tea.MouseMsg{
		X:      8,
		Y:      7,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	}
	updated, cmd = m3.Update(releaseMsg)
	m4 := updated.(Model)
	if m4.selection.active {
		t.Fatal("expected selection.active = false after release")
	}
	if !m4.selection.finished {
		t.Fatal("expected selection.finished = true after release")
	}

	// Verify selected text is non-empty.
	text := m4.selection.selectedText(m4.plainLines)
	if text == "" {
		t.Fatal("expected non-empty selected text")
	}
	t.Logf("selected text preview: %q", text[:min(len(text), 40)])

	// Step 4: Ctrl+C — copy to clipboard.
	updated, cmd = m4.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m5 := updated.(Model)
	if m5.selection.finished {
		t.Fatal("expected selection to be cleared after copy")
	}
	// Verify a "Copied to clipboard" message was appended.
	found := false
	for _, msg := range m5.messages {
		if msg.role == "system" && strings.Contains(msg.content, "Copied to clipboard") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'Copied to clipboard' system message")
	}
	if cmd == nil {
		t.Fatal("expected a cmd for clipboard copy")
	}
}

// TestUpdateMouseSelection_NoSelectionOnEmptyViewport verifies no crash with empty content.
func TestUpdateMouseSelection_EmptyViewport(t *testing.T) {
	m := NewModel(&config.AgentConfig{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, tuiStylesForTest())
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(80, 20)
	m.viewport.SetContent("")
	m.refreshViewport()

	pressMsg := tea.MouseMsg{
		X:      3,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	updated, _ := m.Update(pressMsg)
	// Should not panic. Selection may or may not activate depending on content.
	_ = updated.(Model)
}
