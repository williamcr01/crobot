package web

import (
	"testing"
)

func TestExaToSearchResponse(t *testing.T) {
	p := &exaProvider{}

	er := &exaResponse{
		Results: []exaResult{
			{
				Title:      "Exa Result",
				URL:        "https://example.com",
				Text:       "Full text content of the page.",
				Highlights: []string{"Highlight one", "Highlight two"},
				Summary:    "Summary text",
			},
		},
	}

	resp := p.toSearchResponse(er)

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Exa Result" {
		t.Errorf("unexpected title: %q", resp.Results[0].Title)
	}
	// Text takes precedence over highlights and summary.
	if resp.Results[0].Snippet != "Full text content of the page." {
		t.Errorf("expected text content as snippet, got %q", resp.Results[0].Snippet)
	}
}

func TestExaToSearchResponse_FallbackToHighlight(t *testing.T) {
	p := &exaProvider{}

	er := &exaResponse{
		Results: []exaResult{
			{
				Title:      "No Text",
				URL:        "https://example.com",
				Text:       "",
				Highlights: []string{"First highlight"},
				Summary:    "Summary",
			},
		},
	}

	resp := p.toSearchResponse(er)

	if resp.Results[0].Snippet != "First highlight" {
		t.Errorf("expected highlight fallback, got %q", resp.Results[0].Snippet)
	}
}

func TestExaToSearchResponse_FallbackToSummary(t *testing.T) {
	p := &exaProvider{}

	er := &exaResponse{
		Results: []exaResult{
			{
				Title:   "Summary Only",
				URL:     "https://example.com",
				Summary: "Only summary available",
			},
		},
	}

	resp := p.toSearchResponse(er)

	if resp.Results[0].Snippet != "Only summary available" {
		t.Errorf("expected summary fallback, got %q", resp.Results[0].Snippet)
	}
}

func TestExaToSearchResponse_Empty(t *testing.T) {
	p := &exaProvider{}

	er := &exaResponse{}
	resp := p.toSearchResponse(er)

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestExaStartDate(t *testing.T) {
	// Just verify format and rough correctness.
	tests := []struct {
		filter string
	}{
		{"day"},
		{"week"},
		{"month"},
		{"year"},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			got := exaStartDate(tt.filter)
			if got == "" {
				t.Error("expected non-empty date")
			}
			// Should be ISO 8601 date format.
			if len(got) != 10 || got[4] != '-' || got[7] != '-' {
				t.Errorf("expected YYYY-MM-DD format, got %q", got)
			}
		})
	}

	t.Run("invalid", func(t *testing.T) {
		if got := exaStartDate("invalid"); got != "" {
			t.Errorf("expected empty for invalid filter, got %q", got)
		}
	})
}

func TestExaName(t *testing.T) {
	p := &exaProvider{}
	if p.Name() != "exa" {
		t.Errorf("expected exa, got %q", p.Name())
	}
}
