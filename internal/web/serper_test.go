package web

import (
	"testing"
)

func TestSerperToSearchResponse_Organic(t *testing.T) {
	p := &serperProvider{}

	sr := &serperResponse{
		Organic: []serperResult{
			{Title: "First Result", Link: "https://example.com/1", Snippet: "First snippet"},
			{Title: "Second Result", Link: "https://example.com/2", Snippet: "Second snippet"},
		},
	}

	resp := p.toSearchResponse("test query", sr)

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	if resp.Results[0].Title != "First Result" {
		t.Errorf("unexpected title: %q", resp.Results[0].Title)
	}
	if resp.Results[0].URL != "https://example.com/1" {
		t.Errorf("unexpected URL: %q", resp.Results[0].URL)
	}
	if resp.Results[0].Snippet != "First snippet" {
		t.Errorf("unexpected snippet: %q", resp.Results[0].Snippet)
	}
}

func TestSerperToSearchResponse_AnswerBox(t *testing.T) {
	p := &serperProvider{}

	sr := &serperResponse{
		Organic: []serperResult{
			{Title: "Result", Link: "https://example.com", Snippet: "..."},
		},
		AnswerBox: &serperAnswerBox{
			Title:  "Featured Snippet",
			Answer: "The answer is 42.",
			Link:   "https://example.com/answer",
		},
	}

	resp := p.toSearchResponse("test query", sr)

	if resp.Answer != "The answer is 42." {
		t.Errorf("expected answer from answer box, got %q", resp.Answer)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 organic result, got %d", len(resp.Results))
	}
}

func TestSerperToSearchResponse_KnowledgeGraph(t *testing.T) {
	p := &serperProvider{}

	sr := &serperResponse{
		KnowledgeGraph: &serperKnowledgeGraph{
			Title:       "Go Programming Language",
			Description: "Go is a statically typed, compiled programming language designed at Google.",
			Link:        "https://go.dev",
		},
	}

	resp := p.toSearchResponse("golang", sr)

	if resp.Answer != "Go Programming Language: Go is a statically typed, compiled programming language designed at Google." {
		t.Errorf("unexpected answer: %q", resp.Answer)
	}
	// When there are no organic results, the knowledge graph link becomes a fallback result.
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 fallback result from knowledge graph, got %d", len(resp.Results))
	}
	if resp.Results[0].URL != "https://go.dev" {
		t.Errorf("expected knowledge graph link as result, got %q", resp.Results[0].URL)
	}
}

func TestSerperToSearchResponse_Both(t *testing.T) {
	p := &serperProvider{}

	sr := &serperResponse{
		Organic: []serperResult{
			{Title: "Web Result", Link: "https://example.com", Snippet: "A web page"},
		},
		AnswerBox: &serperAnswerBox{
			Answer: "Direct answer text",
		},
		KnowledgeGraph: &serperKnowledgeGraph{
			Title:       "Topic",
			Description: "Topic description.",
		},
	}

	resp := p.toSearchResponse("test query", sr)

	expected := "Direct answer text\n\nTopic: Topic description."
	if resp.Answer != expected {
		t.Errorf("unexpected answer: %q", resp.Answer)
	}
}

func TestSerperToSearchResponse_Empty(t *testing.T) {
	p := &serperProvider{}

	sr := &serperResponse{}
	resp := p.toSearchResponse("empty", sr)

	if resp.Answer != "" {
		t.Errorf("expected empty answer, got %q", resp.Answer)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestSerperName(t *testing.T) {
	p := &serperProvider{}
	if p.Name() != "serper" {
		t.Errorf("expected serper, got %q", p.Name())
	}
}
