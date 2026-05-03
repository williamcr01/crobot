package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const exaEndpoint = "https://api.exa.ai/search"

// exaProvider implements SearchProvider for Exa API.
// Docs: https://docs.exa.ai/reference/search
type exaProvider struct {
	apiKey string
	client *http.Client
}

func newExaProvider(apiKey string) *exaProvider {
	return &exaProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *exaProvider) Name() ResolvedProvider { return "exa" }

type exaRequest struct {
	Query              string        `json:"query"`
	NumResults         int           `json:"numResults"`
	Type               string        `json:"type"`
	UseAutoprompt      bool          `json:"useAutoprompt"`
	Contents           *exaContents  `json:"contents"`
	StartPublishedDate string        `json:"startPublishedDate,omitempty"`
	IncludeDomains     []string      `json:"includeDomains,omitempty"`
	ExcludeDomains     []string      `json:"excludeDomains,omitempty"`
}

type exaContents struct {
	Text       bool `json:"text"`
	Highlights bool `json:"highlights,omitempty"`
}

type exaResponse struct {
	RequestID string       `json:"requestId"`
	Results   []exaResult  `json:"results"`
}

type exaResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate"`
	Author        string   `json:"author"`
	Text          string   `json:"text"`
	Highlights    []string `json:"highlights"`
	Summary       string   `json:"summary"`
}

func (p *exaProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	num := opts.NumResults
	if num <= 0 {
		num = 5
	}
	if num > 20 {
		num = 20
	}

	includeDomains, excludeDomains := splitDomains(opts.DomainFilter)

	req := exaRequest{
		Query:         query,
		NumResults:    num,
		Type:          "auto",
		UseAutoprompt: true,
		Contents: &exaContents{
			Text:       true,
			Highlights: true,
		},
		IncludeDomains: includeDomains,
		ExcludeDomains: excludeDomains,
	}

	if opts.RecencyFilter != "" {
		req.StartPublishedDate = exaStartDate(opts.RecencyFilter)
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("exa marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, exaEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("exa create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("exa request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("exa returned %d: %s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var er exaResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("exa decode: %w", err)
	}

	return p.toSearchResponse(&er), nil
}

func (p *exaProvider) toSearchResponse(er *exaResponse) *SearchResponse {
	resp := &SearchResponse{}

	for _, r := range er.Results {
		// Use text content as the primary snippet. Fall back to highlights if
		// no text content is available, then to summary.
		snippet := r.Text
		if snippet == "" && len(r.Highlights) > 0 {
			snippet = r.Highlights[0]
		}
		if snippet == "" {
			snippet = r.Summary
		}

		resp.Results = append(resp.Results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		})
	}

	return resp
}

// exaStartDate returns an ISO 8601 date string for the recency filter.
func exaStartDate(filter string) string {
	now := time.Now()
	var d time.Time
	switch filter {
	case "day":
		d = now.AddDate(0, 0, -1)
	case "week":
		d = now.AddDate(0, 0, -7)
	case "month":
		d = now.AddDate(0, -1, 0)
	case "year":
		d = now.AddDate(-1, 0, 0)
	default:
		return ""
	}
	return d.Format("2006-01-02")
}

var _ SearchProvider = (*exaProvider)(nil)
