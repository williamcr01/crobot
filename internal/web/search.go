package web

import (
	"context"
	"fmt"
)

// SearchOptions configures a search request.
type SearchOptions struct {
	NumResults     int      // Results per query (default 5, max 20)
	RecencyFilter  string   // day, week, month, year
	DomainFilter   []string // Include/exclude domains (prefix with - to exclude)
	IncludeContent bool     // Fetch full page content
	Signal         <-chan struct{}
}

// NoProviderError is returned when no search provider is configured.
type NoProviderError struct{}

func (e *NoProviderError) Error() string {
	return `No search provider configured. Add an API key to ~/.crobot/web-search.json:

{
  "exaApiKey": "exa-...",
  "perplexityApiKey": "pplx-...",
  "geminiApiKey": "AIza...",
  "braveApiKey": "BSA-...",
  "tavilyApiKey": "tvly-...",
  "serperApiKey": "..."
}

Supported providers (at least one required):
  - exa        https://exa.ai
  - perplexity https://perplexity.ai
  - gemini     https://aistudio.google.com  (free tier: 1,500 req/day)
  - brave      https://brave.com/search/api (free tier: 2,000/month)
  - tavily     https://tavily.com           (free tier: 1,000/month)
  - serper     https://serper.dev           (2,500 free on signup)`
}

// SearchProvider abstracts a single search backend.
type SearchProvider interface {
	// Name returns the provider identifier (exa, perplexity, gemini, brave, tavily, serper).
	Name() ResolvedProvider
	// Search performs a search and returns synthesized results.
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error)
}

// Search executes a search using the configured provider. Falls back through
// available providers if the requested one fails.
func Search(ctx context.Context, cfg *Config, storage *Storage, query string, opts SearchOptions) (*SearchResponse, error) {
	provider, ok := cfg.ResolveProvider(opts.Provider())
	if !ok {
		return nil, &NoProviderError{}
	}

	var lastErr error
	for _, p := range availableProviders(cfg, provider) {
		resp, err := p.Search(ctx, query, opts)
		if err == nil {
			resp.Provider = string(p.Name())
			return resp, nil
		}
		if isAbortError(err) {
			return nil, err
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all search providers failed: %w", lastErr)
	}
	return nil, &NoProviderError{}
}

// availableProviders returns the ordered list of providers to try,
// starting with the requested one and falling through remaining available ones.
func availableProviders(cfg *Config, requested ResolvedProvider) []SearchProvider {
	var providers []SearchProvider

	// Add requested provider first if available.
	p := makeProvider(requested, cfg.apiKey(requested))
	if p != nil {
		providers = append(providers, p)
	}

	// Add remaining available providers in priority order.
	for _, rp := range cfg.ProviderPriority() {
		if rp == requested {
			continue
		}
		p := makeProvider(rp, cfg.apiKey(rp))
		if p != nil {
			providers = append(providers, p)
		}
	}

	return providers
}

func makeProvider(name ResolvedProvider, apiKey string) SearchProvider {
	if apiKey == "" {
		return nil
	}
	switch name {
	case "exa":
		return newExaProvider(apiKey)
	case "perplexity":
		return newPerplexityProvider(apiKey)
	case "gemini":
		return newGeminiSearchProvider(apiKey)
	case "brave":
		return newBraveProvider(apiKey)
	case "tavily":
		return newTavilyProvider(apiKey)
	case "serper":
		return newSerperProvider(apiKey)
	}
	return nil
}

// Provider returns the requested provider from search options.
func (o SearchOptions) Provider() string {
	// This default is handled by ResolveProvider, but we include it for completeness.
	return ""
}

func isAbortError(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation or deadline exceeded.
	return err == context.Canceled || err == context.DeadlineExceeded
}
