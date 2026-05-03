package web

import (
	"context"
	"fmt"
)

// perplexityProvider implements SearchProvider for Perplexity API.
type perplexityProvider struct {
	apiKey string
}

func newPerplexityProvider(apiKey string) *perplexityProvider {
	return &perplexityProvider{apiKey: apiKey}
}

func (p *perplexityProvider) Name() ResolvedProvider { return "perplexity" }

func (p *perplexityProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	return nil, fmt.Errorf("perplexity: not yet implemented")
}
