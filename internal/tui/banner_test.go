package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBannerLogoLinesHaveEqualVisualWidth(t *testing.T) {
	lines := strings.Split(strings.Trim(logo, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("expected logo lines")
	}

	want := lipgloss.Width(lines[0])
	for i, line := range lines[1:] {
		if got := lipgloss.Width(line); got != want {
			t.Fatalf("logo line %d width = %d, want %d", i+2, got, want)
		}
	}
}

func TestCenteredBannerPreservesLogoLeftEdges(t *testing.T) {
	centered := centerContent(Render("test/model"), 80)
	lines := strings.Split(centered, "\n")
	if len(lines) < 6 {
		t.Fatalf("expected rendered logo lines, got %d", len(lines))
	}

	logoLines := strings.Split(strings.Trim(logo, "\n"), "\n")
	for i, original := range logoLines {
		want := strings.IndexFunc(original, func(r rune) bool { return r != ' ' })
		got := strings.IndexFunc(stripANSI(lines[i]), func(r rune) bool { return r != ' ' })
		if got == -1 {
			t.Fatalf("centered logo line %d has no visible content", i+1)
		}
		if i == 0 {
			basePad := got - want
			for j, other := range logoLines[1:] {
				otherWant := strings.IndexFunc(other, func(r rune) bool { return r != ' ' })
				otherGot := strings.IndexFunc(stripANSI(lines[j+1]), func(r rune) bool { return r != ' ' })
				if otherGot-otherWant != basePad {
					t.Fatalf("centered logo line %d pad = %d, want %d", j+2, otherGot-otherWant, basePad)
				}
			}
			break
		}
	}
}
