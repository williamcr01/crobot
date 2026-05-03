package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const serperEndpoint = "https://google.serper.dev/search"

// serperProvider implements SearchProvider for Serper (Google Search) API.
// Docs: https://serper.dev
type serperProvider struct {
	apiKey string
}

func newSerperProvider(apiKey string) *serperProvider {
	return &serperProvider{apiKey: apiKey}
}

func (p *serperProvider) Name() ResolvedProvider { return "serper" }

type serperRequest struct {
	Query string `json:"q"`
	Num   int    `json:"num,omitempty"`
}

type serperResponse struct {
	Organic        []serperResult       `json:"organic"`
	KnowledgeGraph *serperKnowledgeGraph `json:"knowledgeGraph"`
	AnswerBox      *serperAnswerBox     `json:"answerBox"`
}

type serperResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type serperKnowledgeGraph struct {
	Title       string `json:"title"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Link        string `json:"link,omitempty"`
}

type serperAnswerBox struct {
	Title   string `json:"title"`
	Answer  string `json:"answer"`
	Snippet string `json:"snippet"`
	Link    string `json:"link,omitempty"`
}

func (p *serperProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	num := opts.NumResults
	if num <= 0 {
		num = 5
	}
	if num > 20 {
		num = 20
	}

	body := serperRequest{
		Query: query,
		Num:   num,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serper marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serperEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("serper create request: %w", err)
	}
	req.Header.Set("X-API-KEY", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serper request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("serper returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var sr serperResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("serper decode: %w", err)
	}

	return p.toSearchResponse(query, &sr), nil
}

func (p *serperProvider) toSearchResponse(query string, sr *serperResponse) *SearchResponse {
	resp := &SearchResponse{}

	// Build an answer from knowledge graph and answer box.
	var answerParts []string

	if sr.AnswerBox != nil {
		ab := sr.AnswerBox
		if ab.Answer != "" {
			answerParts = append(answerParts, ab.Answer)
		} else if ab.Snippet != "" {
			answerParts = append(answerParts, ab.Snippet)
		}
	}

	if sr.KnowledgeGraph != nil {
		kg := sr.KnowledgeGraph
		if kg.Description != "" {
			if len(answerParts) > 0 {
				answerParts = append(answerParts, "")
			}
			answerParts = append(answerParts, kg.Title+": "+kg.Description)
		}
	}

	// Collect organic results.
	for _, r := range sr.Organic {
		resp.Results = append(resp.Results, SearchResult{
			Title:   r.Title,
			URL:     r.Link,
			Snippet: r.Snippet,
		})
	}

	// If no organic results and no answer/knowledge graph, add knowledge graph link as result.
	if len(resp.Results) == 0 && sr.KnowledgeGraph != nil && sr.KnowledgeGraph.Link != "" {
		resp.Results = append(resp.Results, SearchResult{
			Title:   sr.KnowledgeGraph.Title,
			URL:     sr.KnowledgeGraph.Link,
			Snippet: sr.KnowledgeGraph.Description,
		})
	}

	resp.Answer = joinLines(answerParts...)
	return resp
}

func joinLines(lines ...string) string {
	return strings.Join(lines, "\n")
}

var _ SearchProvider = (*serperProvider)(nil)
