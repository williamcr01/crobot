package web

import (
	"context"
	"fmt"
)

// braveProvider implements SearchProvider for Brave Search API.
type braveProvider struct {
	apiKey string
}

func newBraveProvider(apiKey string) *braveProvider {
	return &braveProvider{apiKey: apiKey}
}

func (p *braveProvider) Name() ResolvedProvider { return "brave" }

func (p *braveProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	return nil, fmt.Errorf("brave: not yet implemented")
}
