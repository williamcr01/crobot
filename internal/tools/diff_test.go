package tools

import (
	"strings"
	"testing"
)

func TestUnifiedDiffTruncates(t *testing.T) {
	var after strings.Builder
	for i := 0; i < maxDiffLines+20; i++ {
		after.WriteString("line\n")
	}
	result := unifiedDiff("/tmp/file", "", after.String())
	if !result.Truncated {
		t.Fatal("expected truncated diff")
	}
	if !strings.Contains(result.Text, "diff truncated") {
		t.Fatalf("expected truncation marker, got %q", result.Text)
	}
}
