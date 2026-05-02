package tui

import (
	"strings"
	"testing"
)

func TestFormatDiffPreviewIncludesDiffLines(t *testing.T) {
	diff := "--- /tmp/a\n+++ /tmp/a\n@@ -1,1 +1,1 @@\n-old\n+new\n same"
	got := formatDiffPreview(diff, 80, 20, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "▌ old") || !strings.Contains(stripped, "▌ new") {
		t.Fatalf("expected rendered diff lines in output: %q", stripped)
	}
	if strings.Contains(stripped, "--- /tmp/a") || strings.Contains(stripped, "@@ -1,1") {
		t.Fatalf("expected rendered view to hide raw diff headers: %q", stripped)
	}
}

func TestIsDiffOutputOnlyForFileTools(t *testing.T) {
	diff := "--- /tmp/a\n+++ /tmp/a\n-old\n+new\n"
	if !isDiffOutput("file_edit", diff) {
		t.Fatal("expected file_edit diff output")
	}
	if !isDiffOutput("file_write", diff) {
		t.Fatal("expected file_write diff output")
	}
	if isDiffOutput("bash", diff) {
		t.Fatal("did not expect bash diff output")
	}
}

func TestFormatDiffPreviewCollapsed(t *testing.T) {
	diff := "--- /tmp/a\n+++ /tmp/a\n@@ -1,3 +1,3 @@\n-a\n+b\n c"
	got := formatDiffPreview(diff, 80, 3, true, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "ctrl+o to expand") {
		t.Fatalf("expected collapsed hint: %q", stripped)
	}
}

func TestFormatDiffPreviewShowsSummaryAndLineNumbers(t *testing.T) {
	diff := "--- /tmp/a\n+++ /tmp/a\n@@ -7,2 +7,2 @@\n-old\n+new"
	got := formatDiffPreview(diff, 80, 20, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "diff +1 -1") {
		t.Fatalf("expected diff summary: %q", stripped)
	}
	if !strings.Contains(stripped, "   7      ▌ old") || !strings.Contains(stripped, "        7 ▌ new") {
		t.Fatalf("expected old/new line numbers: %q", stripped)
	}
}

func TestFormatDiffPreviewWideUsesSplitLayout(t *testing.T) {
	diff := "--- /tmp/a\n+++ /tmp/a\n@@ -1,2 +1,2 @@\n-old\n+new\n same"
	got := formatDiffPreview(diff, 100, 20, false, testStyles)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, " │ ") {
		t.Fatalf("expected split separator: %q", stripped)
	}
	if !strings.Contains(stripped, "   1 ▌ old") || !strings.Contains(stripped, "   1 ▌ new") {
		t.Fatalf("expected paired remove/add cells: %q", stripped)
	}
}
