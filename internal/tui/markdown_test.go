package tui

import (
	"strings"
	"testing"

	gast "github.com/yuin/goldmark/extension/ast"
)

func TestRenderMarkdown_Bold(t *testing.T) {
	got := RenderMarkdown("**bold text** here", 80)
	if !strings.Contains(stripANSI(got), "bold text") {
		t.Fatalf("expected bold text in output: %q", got)
	}
}

func TestRenderMarkdown_Italic(t *testing.T) {
	got := RenderMarkdown("*italic text* here", 80)
	if !strings.Contains(stripANSI(got), "italic text") {
		t.Fatalf("expected italic text in output: %q", got)
	}
}

func TestRenderMarkdown_InlineCode(t *testing.T) {
	got := RenderMarkdown("use `fmt.Println()` to log", 80)
	if !strings.Contains(stripANSI(got), "fmt.Println()") {
		t.Fatalf("expected inline code: %q", got)
	}
}

func TestRenderMarkdown_Strikethrough(t *testing.T) {
	got := RenderMarkdown("~~old text~~ new", 80)
	if !strings.Contains(stripANSI(got), "old text") {
		t.Fatalf("expected strikethrough text: %q", got)
	}
}

func TestRenderMarkdown_Link(t *testing.T) {
	got := RenderMarkdown("check [the docs](https://example.com) now", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "the docs") {
		t.Fatalf("expected link text: %q", stripped)
	}
	if strings.Contains(stripped, "https://example.com") {
		t.Fatalf("URL should not appear inline: %q", stripped)
	}
}

func TestRenderMarkdown_Image(t *testing.T) {
	got := RenderMarkdown("see ![diagram](img.png) here", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "[img:") {
		t.Fatalf("expected image placeholder: %q", stripped)
	}
}

func TestRenderMarkdown_Heading(t *testing.T) {
	got := RenderMarkdown("# Title\n\ncontent", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "Title") {
		t.Fatalf("expected heading text: %q", stripped)
	}
}

func TestRenderMarkdown_HeadingLevels(t *testing.T) {
	tests := []struct {
		input string
		text  string
	}{
		{"# H1", "H1"},
		{"## H2", "H2"},
		{"### H3", "H3"},
		{"#### H4", "H4"},
	}
	for _, tc := range tests {
		got := RenderMarkdown(tc.input, 80)
		stripped := stripANSI(got)
		if !strings.Contains(stripped, tc.text) {
			t.Errorf("heading %q not found in: %q", tc.input, stripped)
		}
	}
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	got := RenderMarkdown("```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "func main()") {
		t.Fatalf("expected code in output: %q", stripped)
	}
}

func TestRenderMarkdown_UnorderedList(t *testing.T) {
	got := RenderMarkdown("- item one\n- item two", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "item one") || !strings.Contains(stripped, "item two") {
		t.Fatalf("expected list items: %q", stripped)
	}
}

func TestRenderMarkdown_OrderedList(t *testing.T) {
	got := RenderMarkdown("1. first\n2. second", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "first") || !strings.Contains(stripped, "second") {
		t.Fatalf("expected ordered list items: %q", stripped)
	}
}

func TestRenderMarkdown_TaskList(t *testing.T) {
	got := RenderMarkdown("- [ ] pending\n- [x] completed", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "pending") || !strings.Contains(stripped, "completed") {
		t.Fatalf("expected task items: %q", stripped)
	}
}

func TestRenderMarkdown_Blockquote(t *testing.T) {
	got := RenderMarkdown("> quoted text\n> more quote", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "quoted text") {
		t.Fatalf("expected blockquote content: %q", stripped)
	}
}

func TestRenderMarkdown_HorizontalRule(t *testing.T) {
	got := RenderMarkdown("before\n\n---\n\nafter", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "before") || !strings.Contains(stripped, "after") {
		t.Fatalf("expected HR surrounded by text: %q", stripped)
	}
}

func TestRenderMarkdown_Table(t *testing.T) {
	got := RenderMarkdown("| Name | Value |\n|------|-------|\n| foo  | bar   |", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "Name") || !strings.Contains(stripped, "foo") || !strings.Contains(stripped, "bar") {
		t.Fatalf("expected table content: %q", stripped)
	}
}

func TestRenderMarkdown_TableAlignments(t *testing.T) {
	got := RenderMarkdown("| L | C | R |\n|---|:--:|--:|\n| a | b | c |", 80)
	stripped := stripANSI(got)
	for _, s := range []string{"a", "b", "c"} {
		if !strings.Contains(stripped, s) {
			t.Fatalf("expected cell %q in table: %q", s, stripped)
		}
	}
}

func TestRenderMarkdown_NestedList(t *testing.T) {
	got := RenderMarkdown("- parent\n  - child\n  - child2\n- parent2", 80)
	stripped := stripANSI(got)
	for _, s := range []string{"parent", "child", "child2", "parent2"} {
		if !strings.Contains(stripped, s) {
			t.Fatalf("expected %q in nested list: %q", s, stripped)
		}
	}
}

func TestRenderMarkdown_HTMLTags(t *testing.T) {
	got := RenderMarkdown("<strong>bold</strong> and <em>italic</em> and <code>code</code>", 80)
	stripped := stripANSI(got)
	for _, s := range []string{"bold", "italic", "code"} {
		if !strings.Contains(stripped, s) {
			t.Fatalf("expected %q from HTML tag: %q", s, stripped)
		}
	}
}

func TestRenderMarkdown_StripsOtherHTML(t *testing.T) {
	got := RenderMarkdown("<div>hello <span>world</span></div>", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "hello") || !strings.Contains(stripped, "world") {
		t.Fatalf("expected stripped HTML to keep text: %q", stripped)
	}
	if strings.Contains(stripped, "<div>") || strings.Contains(stripped, "<span>") {
		t.Fatalf("HTML tags should be stripped: %q", stripped)
	}
}

func TestRenderMarkdown_MixedBoldItalic(t *testing.T) {
	got := RenderMarkdown("**bold and *italic* together**", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "bold and") || !strings.Contains(stripped, "italic") || !strings.Contains(stripped, "together") {
		t.Fatalf("expected nested bold/italic: %q", stripped)
	}
}

func TestRenderMarkdown_EmptyString(t *testing.T) {
	got := RenderMarkdown("", 80)
	if got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
}

func TestRenderMarkdown_WidthWrapping(t *testing.T) {
	got := RenderMarkdown("this is a very long paragraph that should wrap at the specified width", 30)
	stripped := stripANSI(got)
	for _, line := range strings.Split(stripped, "\n") {
		if len(line) > 30 {
			t.Fatalf("line exceeds width 30 (len=%d): %q", len(line), line)
		}
	}
}

func TestRenderMarkdown_KeywordBold(t *testing.T) {
	got := RenderMarkdown("this is __also bold__ text", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "also bold") {
		t.Fatalf("expected __bold__: %q", stripped)
	}
}

func TestRenderMarkdown_UnderscoreItalic(t *testing.T) {
	got := RenderMarkdown("this is _italic_ text", 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "italic") {
		t.Fatalf("expected _italic_: %q", stripped)
	}
}

// --- Markdown helper function tests ---

func TestShrinkColumns(t *testing.T) {
	// Total fits within available
	result := shrinkColumns([]int{10, 20, 30}, 100)
	if len(result) != 3 || result[0] != 10 || result[1] != 20 || result[2] != 30 {
		t.Fatalf("expected original widths, got %v", result)
	}

	// Total exceeds available
	result2 := shrinkColumns([]int{50, 50, 50}, 60)
	total := 0
	for _, w := range result2 {
		total += w
	}
	if total > 66 { // 60 + possible rounding
		t.Fatalf("expected shrunk total <= 66, got %d: %v", total, result2)
	}

	// Single column
	result3 := shrinkColumns([]int{100}, 30)
	if result3[0] < 3 || result3[0] > 30 {
		t.Fatalf("expected shrunk single column between 3-30, got %d", result3[0])
	}
}

func TestStripStyles(t *testing.T) {
	if got := stripStyles(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	if got := stripStyles("hello"); got != "hello" {
		t.Fatalf("expected unchanged, got %q", got)
	}

	// CSI sequence
	if got := stripStyles("\x1b[1mhello\x1b[0m"); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}

	// OSC sequence
	if got := stripStyles("\x1b]0;title\x07body"); got != "body" {
		t.Fatalf("expected 'body', got %q", got)
	}

	// Unknown escape (just ESC + one char)
	if got := stripStyles("\x1bXabc"); got != "abc" {
		t.Fatalf("expected 'abc', got %q", got)
	}
}

func TestDisplayWidth(t *testing.T) {
	if got := displayWidth("hello"); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}

	if got := displayWidth("\x1b[1mhello\x1b[0m"); got != 5 {
		t.Fatalf("expected 5 with styles, got %d", got)
	}

	if got := displayWidth(""); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}

	// Multi-byte characters
	if got := displayWidth("hello\u4e16"); got != 7 { // 5 + 2 for CJK char
		t.Fatalf("expected 7 with CJK, got %d", got)
	}
}

func TestLongestLine(t *testing.T) {
	if got := longestLine(""); got != 0 {
		t.Fatalf("expected 0 for empty, got %d", got)
	}

	if got := longestLine("hello\nworld!"); got != 6 {
		t.Fatalf("expected 6 for 'world!', got %d", got)
	}

	if got := longestLine("short"); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestWrapToLines(t *testing.T) {
	lines := wrapToLines("hello world foo bar", 10)
	for _, line := range lines {
		if displayWidth(line) > 10 {
			t.Fatalf("line exceeds width 10: %q (len=%d)", line, displayWidth(line))
		}
	}

	if got := wrapToLines("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("expected one empty line, got %v", got)
	}
}

func TestPadCell(t *testing.T) {
	// Left align (default)
	got := padCell("hello", 10, alignLeft)
	if len(got) != 10 || !strings.HasPrefix(got, "hello") {
		t.Fatalf("expected 'hello' left-padded to 10: %q", got)
	}

	// Right align
	got2 := padCell("hello", 10, alignRight)
	if len(got2) != 10 || !strings.HasSuffix(got2, "hello") {
		t.Fatalf("expected 'hello' right-padded to 10: %q", got2)
	}

	// Center align
	got3 := padCell("hi", 10, alignCenter)
	if len(got3) != 10 {
		t.Fatalf("expected length 10, got %d: %q", len(got3), got3)
	}

	// Text exceeds width (truncation)
	got4 := padCell("hello world!", 5, alignLeft)
	if displayWidth(got4) > 5 {
		t.Fatalf("expected truncated, got len %d: %q", displayWidth(got4), got4)
	}
}

func TestPadRow(t *testing.T) {
	row := []string{"a", "b"}
	result := padRow(row, 4)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
	if result[2] != "" || result[3] != "" {
		t.Fatalf("expected empty strings for padded elements: %v", result)
	}

	// No padding needed
	result2 := padRow(row, 2)
	if len(result2) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result2))
	}
}

func TestAlignFor(t *testing.T) {
	aligns := []alignment{alignLeft, alignCenter, alignRight}

	if got := alignFor(aligns, 0); got != alignLeft {
		t.Fatalf("expected left, got %d", got)
	}
	if got := alignFor(aligns, 1); got != alignCenter {
		t.Fatalf("expected center, got %d", got)
	}
	if got := alignFor(aligns, 2); got != alignRight {
		t.Fatalf("expected right, got %d", got)
	}

	// Fallback for index out of range
	if got := alignFor(aligns, 10); got != alignLeft {
		t.Fatalf("expected left fallback, got %d", got)
	}

	// Empty aligns
	if got := alignFor(nil, 0); got != alignLeft {
		t.Fatalf("expected left for empty, got %d", got)
	}
}

func TestCollectText(t *testing.T) {
	// collectText works with rendered markdown output
	output := RenderMarkdown("hello **world**", 80)
	// Just verify it doesn't panic and the text appears
	if !strings.Contains(stripANSI(output), "hello") {
		t.Fatalf("expected 'hello' in output: %q", output)
	}
}

func TestStripHTMLTags(t *testing.T) {
	if got := stripHTMLTags("<div>content</div>"); got != "content" {
		t.Fatalf("expected 'content', got %q", got)
	}

	if got := stripHTMLTags("no tags"); got != "no tags" {
		t.Fatalf("expected unchanged, got %q", got)
	}

	if got := stripHTMLTags(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestTableAlign(t *testing.T) {
	_ = tableAlign(gast.AlignCenter)
	_ = tableAlign(gast.AlignRight)
	_ = tableAlign(gast.AlignLeft)
}

func TestRenderTableTopBorder(t *testing.T) {
	got := renderTableTopBorder([]int{10, 20})
	if !strings.Contains(got, "┌") || !strings.Contains(got, "┬") || !strings.Contains(got, "┐") {
		t.Fatalf("expected table top border: %q", got)
	}
}

func TestRenderTableSepBorder(t *testing.T) {
	got := renderTableSepBorder([]int{10, 20})
	if !strings.Contains(got, "├") || !strings.Contains(got, "┼") || !strings.Contains(got, "┤") {
		t.Fatalf("expected table sep border: %q", got)
	}
}

func TestRenderTableBotBorder(t *testing.T) {
	got := renderTableBotBorder([]int{10, 20})
	if !strings.Contains(got, "└") || !strings.Contains(got, "┴") || !strings.Contains(got, "┘") {
		t.Fatalf("expected table bot border: %q", got)
	}
}

func TestRenderTableFrame_Empty(t *testing.T) {
	got := renderTableFrame([]int{}, "┌", "┬", "┐", "─")
	if got != "┌┐" {
		t.Fatalf("expected '┌┐', got %q", got)
	}
}

