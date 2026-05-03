package web

import (
	"testing"

	"google.golang.org/genai"
)

func TestParseSearchResponse_Empty(t *testing.T) {
	p := &geminiSearchProvider{}

	t.Run("nil response", func(t *testing.T) {
		resp, err := p.parseSearchResponse("test", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Answer != "" {
			t.Errorf("expected empty answer, got %q", resp.Answer)
		}
		if len(resp.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(resp.Results))
		}
	})

	t.Run("no candidates", func(t *testing.T) {
		resp, err := p.parseSearchResponse("test", &genai.GenerateContentResponse{})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Answer != "" {
			t.Errorf("expected empty answer, got %q", resp.Answer)
		}
	})

	t.Run("nil content", func(t *testing.T) {
		resp, err := p.parseSearchResponse("test", &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{Content: nil}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Answer != "" {
			t.Errorf("expected empty answer, got %q", resp.Answer)
		}
	})
}

func TestParseSearchResponse_WithAnswer(t *testing.T) {
	p := &geminiSearchProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "This is the answer.", Thought: false},
						{Text: "internal thinking", Thought: true},
						{Text: " Additional context.", Thought: false},
					},
				},
				GroundingMetadata: &genai.GroundingMetadata{
					GroundingChunks: []*genai.GroundingChunk{
						{
							Web: &genai.GroundingChunkWeb{
								Title: "Source One",
								URI:   "https://example.com/one",
							},
						},
						{
							Web: &genai.GroundingChunkWeb{
								Title: "Source Two",
								URI:   "https://example.com/two",
							},
						},
					},
				},
			},
		},
	}

	result, err := p.parseSearchResponse("test", resp)
	if err != nil {
		t.Fatal(err)
	}

	if result.Answer != "This is the answer.\n Additional context." {
		t.Errorf("unexpected answer: %q", result.Answer)
	}

	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}

	if result.Results[0].Title != "Source One" {
		t.Errorf("unexpected title: %q", result.Results[0].Title)
	}
	if result.Results[0].URL != "https://example.com/one" {
		t.Errorf("unexpected URL: %q", result.Results[0].URL)
	}
}

func TestExtractGroundingResults_Deduplicates(t *testing.T) {
	p := &geminiSearchProvider{}

	metadata := &genai.GroundingMetadata{
		GroundingChunks: []*genai.GroundingChunk{
			{Web: &genai.GroundingChunkWeb{Title: "A", URI: "https://example.com/a"}},
			{Web: &genai.GroundingChunkWeb{Title: "A Again", URI: "https://example.com/a"}},
			{Web: &genai.GroundingChunkWeb{Title: "B", URI: "https://example.com/b"}},
		},
	}

	results := p.extractGroundingResults(metadata)
	if len(results) != 2 {
		t.Fatalf("expected 2 deduplicated results, got %d", len(results))
	}
	if results[0].URL != "https://example.com/a" {
		t.Errorf("expected first URL a, got %q", results[0].URL)
	}
	if results[1].URL != "https://example.com/b" {
		t.Errorf("expected second URL b, got %q", results[1].URL)
	}
}

func TestExtractGroundingResults_SkipsEmptyURL(t *testing.T) {
	p := &geminiSearchProvider{}

	metadata := &genai.GroundingMetadata{
		GroundingChunks: []*genai.GroundingChunk{
			{Web: &genai.GroundingChunkWeb{Title: "No URL", URI: ""}},
			{Web: &genai.GroundingChunkWeb{Title: "Has URL", URI: "https://example.com"}},
		},
	}

	results := p.extractGroundingResults(metadata)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].URL != "https://example.com" {
		t.Errorf("unexpected URL: %q", results[0].URL)
	}
}

func TestExtractGroundingResults_NilWeb(t *testing.T) {
	p := &geminiSearchProvider{}

	metadata := &genai.GroundingMetadata{
		GroundingChunks: []*genai.GroundingChunk{
			{Web: nil},
			{Web: &genai.GroundingChunkWeb{Title: "Valid", URI: "https://example.com"}},
		},
	}

	results := p.extractGroundingResults(metadata)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestName(t *testing.T) {
	p := &geminiSearchProvider{}
	if p.Name() != "gemini" {
		t.Errorf("expected gemini, got %q", p.Name())
	}
}
