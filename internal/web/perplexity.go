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

const (
	perplexityEndpoint = "https://api.perplexity.ai/chat/completions"
	perplexityModel    = "sonar"
)

// perplexityProvider implements SearchProvider for Perplexity API.
// Docs: https://docs.perplexity.ai
type perplexityProvider struct {
	apiKey string
	client *http.Client
}

func newPerplexityProvider(apiKey string) *perplexityProvider {
	return &perplexityProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *perplexityProvider) Name() ResolvedProvider { return "perplexity" }

type perplexityMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type perplexityRequest struct {
	Model    string              `json:"model"`
	Messages []perplexityMessage `json:"messages"`
}

type perplexityResponse struct {
	ID        string              `json:"id"`
	Model     string              `json:"model"`
	Citations []string            `json:"citations"`
	Choices   []perplexityChoice  `json:"choices"`
}

type perplexityChoice struct {
	Message perplexityMessage `json:"message"`
}

func (p *perplexityProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	req := perplexityRequest{
		Model: perplexityModel,
		Messages: []perplexityMessage{
			{Role: "system", Content: "You are a precise search assistant. Answer concisely with citations. Be accurate."},
			{Role: "user", Content: query},
		},
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("perplexity marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, perplexityEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("perplexity create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("perplexity request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("perplexity returned %d: %s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var pr perplexityResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("perplexity decode: %w", err)
	}

	return p.toSearchResponse(&pr), nil
}

func (p *perplexityProvider) toSearchResponse(pr *perplexityResponse) *SearchResponse {
	resp := &SearchResponse{}

	// Extract the assistant's answer.
	if len(pr.Choices) > 0 {
		resp.Answer = pr.Choices[0].Message.Content
	}

	// Map citations to results.
	for _, url := range pr.Citations {
		resp.Results = append(resp.Results, SearchResult{
			Title: url,
			URL:   url,
		})
	}

	return resp
}

var _ SearchProvider = (*perplexityProvider)(nil)
