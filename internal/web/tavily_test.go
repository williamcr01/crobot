package web

import (
	"testing"
)

func TestTavilyToSearchResponse(t *testing.T) {
	p := &tavilyProvider{}

	tr := &tavilyResponse{
		Query:  "test",
		Answer: "The answer is 42.",
		Results: []tavilyResult{
			{Title: "Result 1", URL: "https://a.com", Content: "Content A", Score: 0.95},
			{Title: "Result 2", URL: "https://b.com", Content: "Content B", Score: 0.80},
		},
	}

	resp := p.toSearchResponse(tr)

	if resp.Answer != "The answer is 42." {
		t.Errorf("expected answer, got %q", resp.Answer)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Snippet != "Content A" {
		t.Errorf("expected Content A snippet, got %q", resp.Results[0].Snippet)
	}
}

func TestTavilyToSearchResponse_NoAnswer(t *testing.T) {
	p := &tavilyProvider{}

	tr := &tavilyResponse{
		Results: []tavilyResult{
			{Title: "Only Result", URL: "https://c.com", Content: "Content C", Score: 0.70},
		},
	}

	resp := p.toSearchResponse(tr)

	if resp.Answer != "" {
		t.Errorf("expected no answer, got %q", resp.Answer)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestRecencyDays(t *testing.T) {
	tests := []struct {
		filter string
		want   int
	}{
		{"day", 1},
		{"week", 7},
		{"month", 30},
		{"year", 365},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			got := recencyDays(tt.filter)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSplitDomains(t *testing.T) {
	tests := []struct {
		name        string
		filter      []string
		wantInclude []string
		wantExclude []string
	}{
		{"empty", nil, nil, nil},
		{"include only", []string{"github.com", "docs.rs"}, []string{"github.com", "docs.rs"}, nil},
		{"exclude only", []string{"-reddit.com"}, nil, []string{"reddit.com"}},
		{"mixed", []string{"github.com", "-reddit.com", "stackoverflow.com"}, []string{"github.com", "stackoverflow.com"}, []string{"reddit.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inc, exc := splitDomains(tt.filter)
			if !stringSlicesEqual(inc, tt.wantInclude) {
				t.Errorf("include: got %v, want %v", inc, tt.wantInclude)
			}
			if !stringSlicesEqual(exc, tt.wantExclude) {
				t.Errorf("exclude: got %v, want %v", exc, tt.wantExclude)
			}
		})
	}
}

func TestTavilyName(t *testing.T) {
	p := &tavilyProvider{}
	if p.Name() != "tavily" {
		t.Errorf("expected tavily, got %q", p.Name())
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
