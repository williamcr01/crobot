package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

// braveProvider implements SearchProvider for Brave Search API.
// Docs: https://brave.com/search/api/
type braveProvider struct {
	apiKey  string
	client  *http.Client
}

func newBraveProvider(apiKey string) *braveProvider {
	return &braveProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *braveProvider) Name() ResolvedProvider { return "brave" }

type braveWebResponse struct {
	Web *braveWeb `json:"web"`
}

type braveWeb struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Description  string   `json:"description"`
	ExtraSnippets []string `json:"extra_snippets"`
}

func (p *braveProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	num := opts.NumResults
	if num <= 0 {
		num = 5
	}
	if num > 20 {
		num = 20
	}

	// Build query with domain filters as search operators.
	q := buildBraveQuery(query, opts.DomainFilter)

	params := url.Values{}
	params.Set("q", q)
	params.Set("count", fmt.Sprintf("%d", num))
	params.Set("extra_snippets", "true")

	if opts.RecencyFilter != "" {
		params.Set("freshness", braveRecency(opts.RecencyFilter))
	}

	reqURL := braveEndpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("brave create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("brave returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var bwr braveWebResponse
	if err := json.NewDecoder(resp.Body).Decode(&bwr); err != nil {
		return nil, fmt.Errorf("brave decode: %w", err)
	}

	return p.toSearchResponse(&bwr), nil
}

func (p *braveProvider) toSearchResponse(bwr *braveWebResponse) *SearchResponse {
	resp := &SearchResponse{}

	if bwr.Web == nil {
		return resp
	}

	for _, r := range bwr.Web.Results {
		snippet := r.Description
		// Append extra snippets for richer context.
		if len(r.ExtraSnippets) > 0 {
			extra := strings.Join(r.ExtraSnippets, " ")
			if snippet != "" {
				snippet += " " + extra
			} else {
				snippet = extra
			}
		}

		resp.Results = append(resp.Results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		})
	}

	return resp
}

// buildBraveQuery incorporates domain filters as Brave search operators.
func buildBraveQuery(query string, domainFilter []string) string {
	var parts []string

	for _, d := range domainFilter {
		if strings.HasPrefix(d, "-") {
			parts = append(parts, "-site:"+d[1:])
		} else {
			parts = append(parts, "site:"+d)
		}
	}

	if len(parts) > 0 {
		return query + " " + strings.Join(parts, " ")
	}
	return query
}

// braveRecency maps our recency filter to Brave's freshness parameter.
func braveRecency(filter string) string {
	switch filter {
	case "day":
		return "pd"
	case "week":
		return "pw"
	case "month":
		return "pm"
	case "year":
		return "py"
	default:
		return ""
	}
}

var _ SearchProvider = (*braveProvider)(nil)
