package tui

import (
	"strings"
	"testing"
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
