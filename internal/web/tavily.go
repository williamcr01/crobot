package web

import (
	"context"
	"fmt"
)

// tavilyProvider implements SearchProvider for Tavily Search API.
type tavilyProvider struct {
	apiKey string
}

func newTavilyProvider(apiKey string) *tavilyProvider {
	return &tavilyProvider{apiKey: apiKey}
}

func (p *tavilyProvider) Name() ResolvedProvider { return "tavily" }

func (p *tavilyProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	return nil, fmt.Errorf("tavily: not yet implemented")
}
