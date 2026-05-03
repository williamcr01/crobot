package web

import (
	"context"
	"fmt"
)

// exaProvider implements SearchProvider for Exa API.
type exaProvider struct {
	apiKey string
}

func newExaProvider(apiKey string) *exaProvider {
	return &exaProvider{apiKey: apiKey}
}

func (p *exaProvider) Name() ResolvedProvider { return "exa" }

func (p *exaProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	return nil, fmt.Errorf("exa: not yet implemented")
}
