package web

import (
	"context"
	"testing"
)

func TestExtractYouTubeID(t *testing.T) {
	tests := []struct {
		url   string
		want  string
	}{
		{"https://youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtube.com/v/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtube.com/live/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtube.com/watch?v=dQw4w9WgXcQ&t=30", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?si=abc123", "dQw4w9WgXcQ"},
		// extractYouTubeID doesn't validate the hostname — isYouTubeURL does that first.
		// With no youtube.com/youtu.be prefix, it falls through to the patterns list and returns empty.
		{"https://vimeo.com/123456789", ""},
		{"https://example.com", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := extractYouTubeID(tt.url)
			if got != tt.want {
				t.Errorf("extractYouTubeID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url         string
		wantOwner   string
		wantRepo    string
		wantRef     string
		wantPath    string
		wantKind    string
	}{
		{"https://github.com/owner/repo", "owner", "repo", "", "", "root"},
		{"https://github.com/owner/repo.git", "owner", "repo", "", "", "root"},
		{"https://github.com/owner/repo/blob/main/main.go", "owner", "repo", "main", "main.go", "blob"},
		{"https://github.com/owner/repo/blob/v1.0/src/lib.rs", "owner", "repo", "v1.0", "src/lib.rs", "blob"},
		{"https://github.com/owner/repo/tree/main", "owner", "repo", "main", "", "tree"},
		{"https://github.com/owner/repo/tree/main/src/", "owner", "repo", "main", "src", "tree"},
		{"https://github.com/owner/repo/commit/abc123def456", "owner", "repo", "", "abc123def456", "commit"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, ref, path, kind := parseGitHubURL(tt.url)
			if owner != tt.wantOwner || repo != tt.wantRepo || ref != tt.wantRef || path != tt.wantPath || kind != tt.wantKind {
				t.Errorf("parseGitHubURL(%q) = (%q, %q, %q, %q, %q), want (%q, %q, %q, %q, %q)",
					tt.url, owner, repo, ref, path, kind,
					tt.wantOwner, tt.wantRepo, tt.wantRef, tt.wantPath, tt.wantKind)
			}
		})
	}
}

func TestExtractGitHub_InvalidURL(t *testing.T) {
	_, err := extractGitHub(context.Background(), "https://example.com", false)
	if err == nil {
		t.Error("expected error for invalid GitHub URL")
	}
}

func TestYTMeta(t *testing.T) {
	html := `<html><head><meta name="title" content="Video Title"><meta name="description" content="Video description text"></head></html>`
	title := extractYTMeta(html, "name=\"title\"")
	if title != "Video Title" {
		t.Errorf("expected 'Video Title', got %q", title)
	}
	desc := extractYTMeta(html, "name=\"description\"")
	if desc != "Video description text" {
		t.Errorf("expected 'Video description text', got %q", desc)
	}
}

func TestYTMeta_NotFound(t *testing.T) {
	got := extractYTMeta("<html></html>", "name=\"title\"")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
