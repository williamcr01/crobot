package web

import (
	"context"
	"fmt"
)

// serperProvider implements SearchProvider for Serper (Google Search) API.
type serperProvider struct {
	apiKey string
}

func newSerperProvider(apiKey string) *serperProvider {
	return &serperProvider{apiKey: apiKey}
}

func (p *serperProvider) Name() ResolvedProvider { return "serper" }

func (p *serperProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	return nil, fmt.Errorf("serper: not yet implemented")
}
