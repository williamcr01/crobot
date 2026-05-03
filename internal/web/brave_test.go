package web

import (
	"strings"
	"testing"
)

func TestBraveToSearchResponse(t *testing.T) {
	p := &braveProvider{}

	bwr := &braveWebResponse{
		Web: &braveWeb{
			Results: []braveResult{
				{Title: "First", URL: "https://a.com", Description: "Desc A"},
				{Title: "Second", URL: "https://b.com", Description: "Desc B", ExtraSnippets: []string{"extra1", "extra2"}},
			},
		},
	}

	resp := p.toSearchResponse(bwr)

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	if resp.Results[0].Title != "First" {
		t.Errorf("unexpected title: %q", resp.Results[0].Title)
	}

	// Second result should combine description with extra snippets.
	if !strings.Contains(resp.Results[1].Snippet, "extra1") {
		t.Errorf("expected extra snippets in snippet, got %q", resp.Results[1].Snippet)
	}
}

func TestBraveToSearchResponse_Empty(t *testing.T) {
	p := &braveProvider{}

	t.Run("nil web", func(t *testing.T) {
		bwr := &braveWebResponse{Web: nil}
		resp := p.toSearchResponse(bwr)
		if len(resp.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(resp.Results))
		}
	})

	t.Run("empty results", func(t *testing.T) {
		bwr := &braveWebResponse{Web: &braveWeb{Results: nil}}
		resp := p.toSearchResponse(bwr)
		if len(resp.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(resp.Results))
		}
	})
}

func TestBuildBraveQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		filter  []string
		want    string
	}{
		{"no filter", "golang", nil, "golang"},
		{"include domain", "golang", []string{"github.com"}, "golang site:github.com"},
		{"exclude domain", "golang", []string{"-reddit.com"}, "golang -site:reddit.com"},
		{"both", "rust", []string{"github.com", "-reddit.com"}, "rust site:github.com -site:reddit.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBraveQuery(tt.query, tt.filter)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBraveRecency(t *testing.T) {
	tests := []struct {
		filter string
		want   string
	}{
		{"day", "pd"},
		{"week", "pw"},
		{"month", "pm"},
		{"year", "py"},
		{"", ""},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			got := braveRecency(tt.filter)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBraveName(t *testing.T) {
	p := &braveProvider{}
	if p.Name() != "brave" {
		t.Errorf("expected brave, got %q", p.Name())
	}
}
