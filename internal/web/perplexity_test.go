package web

import (
	"testing"
)

func TestPerplexityToSearchResponse(t *testing.T) {
	p := &perplexityProvider{}

	pr := &perplexityResponse{
		ID:    "resp-1",
		Model: "sonar",
		Citations: []string{
			"https://example.com/1",
			"https://example.com/2",
		},
		Choices: []perplexityChoice{
			{Message: perplexityMessage{Role: "assistant", Content: "The answer is 42 based on the sources."}},
		},
	}

	resp := p.toSearchResponse(pr)

	if resp.Answer != "The answer is 42 based on the sources." {
		t.Errorf("unexpected answer: %q", resp.Answer)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].URL != "https://example.com/1" {
		t.Errorf("unexpected URL: %q", resp.Results[0].URL)
	}
	if resp.Results[1].URL != "https://example.com/2" {
		t.Errorf("unexpected URL: %q", resp.Results[1].URL)
	}
}

func TestPerplexityToSearchResponse_NoChoices(t *testing.T) {
	p := &perplexityProvider{}

	pr := &perplexityResponse{
		Citations: []string{"https://example.com"},
	}

	resp := p.toSearchResponse(pr)

	if resp.Answer != "" {
		t.Errorf("expected empty answer, got %q", resp.Answer)
	}
	// Citations still mapped even without answer text.
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestPerplexityToSearchResponse_NoCitations(t *testing.T) {
	p := &perplexityProvider{}

	pr := &perplexityResponse{
		Choices: []perplexityChoice{
			{Message: perplexityMessage{Role: "assistant", Content: "Answer without citations."}},
		},
	}

	resp := p.toSearchResponse(pr)

	if resp.Answer != "Answer without citations." {
		t.Errorf("unexpected answer: %q", resp.Answer)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestPerplexityToSearchResponse_Empty(t *testing.T) {
	p := &perplexityProvider{}

	pr := &perplexityResponse{}
	resp := p.toSearchResponse(pr)

	if resp.Answer != "" {
		t.Errorf("expected empty answer, got %q", resp.Answer)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestPerplexityName(t *testing.T) {
	p := &perplexityProvider{}
	if p.Name() != "perplexity" {
		t.Errorf("expected perplexity, got %q", p.Name())
	}
}
