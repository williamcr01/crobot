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

const tavilyEndpoint = "https://api.tavily.com/search"

// tavilyProvider implements SearchProvider for Tavily Search API.
// Docs: https://tavily.com
type tavilyProvider struct {
	apiKey string
	client *http.Client
}

func newTavilyProvider(apiKey string) *tavilyProvider {
	return &tavilyProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *tavilyProvider) Name() ResolvedProvider { return "tavily" }

type tavilyRequest struct {
	Query        string `json:"query"`
	SearchDepth  string `json:"search_depth,omitempty"`
	IncludeAnswer bool  `json:"include_answer"`
	MaxResults   int    `json:"max_results,omitempty"`
	Topic        string `json:"topic,omitempty"`
	Days         int    `json:"days,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
}

type tavilyResponse struct {
	Query      string          `json:"query"`
	Answer     string          `json:"answer"`
	Results    []tavilyResult  `json:"results"`
	ResponseTime string        `json:"response_time"`
}

type tavilyResult struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	RawContent *string `json:"raw_content"`
}

func (p *tavilyProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	num := opts.NumResults
	if num <= 0 {
		num = 5
	}
	if num > 20 {
		num = 20
	}

	includeDomains, excludeDomains := splitDomains(opts.DomainFilter)

	req := tavilyRequest{
		Query:         query,
		SearchDepth:   "basic",
		IncludeAnswer: true,
		MaxResults:    num,
		IncludeDomains: includeDomains,
		ExcludeDomains: excludeDomains,
	}

	if opts.RecencyFilter != "" {
		req.Days = recencyDays(opts.RecencyFilter)
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("tavily marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("tavily create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tavily request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tavily returned %d: %s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var tr tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("tavily decode: %w", err)
	}

	return p.toSearchResponse(&tr), nil
}

func (p *tavilyProvider) toSearchResponse(tr *tavilyResponse) *SearchResponse {
	resp := &SearchResponse{
		Answer: tr.Answer,
	}

	for _, r := range tr.Results {
		resp.Results = append(resp.Results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	return resp
}

// recencyDays maps our recency filter to Tavily's days parameter.
func recencyDays(filter string) int {
	switch filter {
	case "day":
		return 1
	case "week":
		return 7
	case "month":
		return 30
	case "year":
		return 365
	default:
		return 0
	}
}

// splitDomains separates domain filters into include and exclude lists.
func splitDomains(filter []string) (include, exclude []string) {
	for _, d := range filter {
		if strings.HasPrefix(d, "-") {
			exclude = append(exclude, d[1:])
		} else {
			include = append(include, d)
		}
	}
	return include, exclude
}

var _ SearchProvider = (*tavilyProvider)(nil)
