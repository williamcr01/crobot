package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"
)

const geminiSearchDefaultModel = "gemini-2.5-flash"

// geminiSearchProvider implements SearchProvider using Gemini's google_search grounding.
type geminiSearchProvider struct {
	apiKey string
}

func newGeminiSearchProvider(apiKey string) *geminiSearchProvider {
	return &geminiSearchProvider{apiKey: apiKey}
}

func (p *geminiSearchProvider) Name() ResolvedProvider { return "gemini" }

func (p *geminiSearchProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  p.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}

	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	config := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	resp, err := client.Models.GenerateContent(ctx, geminiSearchDefaultModel, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini search: %w", err)
	}

	return p.parseSearchResponse(query, resp)
}

func (p *geminiSearchProvider) parseSearchResponse(query string, resp *genai.GenerateContentResponse) (*SearchResponse, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return &SearchResponse{
			Answer:  "",
			Results: nil,
		}, nil
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil {
		return &SearchResponse{
			Answer:  "",
			Results: nil,
		}, nil
	}

	// Extract answer text from non-thought parts.
	var answerParts []string
	for _, part := range candidate.Content.Parts {
		if !part.Thought && part.Text != "" {
			answerParts = append(answerParts, part.Text)
		}
	}
	answer := strings.TrimSpace(strings.Join(answerParts, "\n"))

	// Extract source URLs from grounding metadata.
	results := p.extractGroundingResults(candidate.GroundingMetadata)

	return &SearchResponse{
		Answer:  answer,
		Results: results,
	}, nil
}

func (p *geminiSearchProvider) extractGroundingResults(metadata *genai.GroundingMetadata) []SearchResult {
	if metadata == nil || len(metadata.GroundingChunks) == 0 {
		return nil
	}

	seen := map[string]bool{}
	var results []SearchResult

	for _, chunk := range metadata.GroundingChunks {
		if chunk.Web == nil {
			continue
		}
		title := chunk.Web.Title
		url := chunk.Web.URI

		if url == "" {
			continue
		}

		// Resolve Google grounding redirect URLs.
		if strings.Contains(url, "vertexaisearch.cloud.google.com/grounding-api-redirect") {
			if resolved := resolveRedirect(url); resolved != "" {
				url = resolved
			}
		}

		if seen[url] {
			continue
		}
		seen[url] = true

		results = append(results, SearchResult{
			Title: title,
			URL:   url,
		})
	}

	return results
}

// resolveRedirect follows a single HTTP redirect to get the real URL.
func resolveRedirect(url string) string {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Head(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	return resp.Header.Get("Location")
}

// Ensure SearchProvider interface compliance.
var _ SearchProvider = (*geminiSearchProvider)(nil)
