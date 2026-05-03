package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"crobot/internal/web"
)

const maxInlineContent = 30000

// WebSearchTool creates the web_search tool using the given config and storage.
func WebSearchTool(cfg *web.Config, storage *web.Storage) Tool {
	return Tool{
		Name:        "web_search",
		Description: "Search the web using Perplexity AI, Exa, or Gemini. Returns an AI-synthesized answer with source citations. For comprehensive research, prefer queries (plural) with 2-4 varied angles over a single query — each query gets its own synthesized answer, so varying phrasing and scope gives much broader coverage. When includeContent is true, full page content is fetched in the background. Provider auto-selects: Exa (direct API), then Perplexity, then Gemini API, then Brave, then Tavily, then Serper.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Single search query. For research tasks, prefer 'queries' with multiple varied angles instead.",
				},
				"queries": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Multiple queries searched in sequence, each returning its own synthesized answer. Prefer this for research — vary phrasing, scope, and angle across 2-4 queries to maximize coverage.",
				},
				"numResults": map[string]any{
					"type":        "number",
					"description": "Results per query (default: 5, max: 20)",
				},
				"includeContent": map[string]any{
					"type":        "boolean",
					"description": "Fetch full page content from sources in background",
				},
				"recencyFilter": map[string]any{
					"type":        "string",
					"enum":        []string{"day", "week", "month", "year"},
					"description": "Filter by recency",
				},
				"domainFilter": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Limit to domains (prefix with - to exclude)",
				},
				"provider": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "exa", "perplexity", "gemini", "brave", "tavily", "serper"},
					"description": "Search provider (default: auto)",
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			queryList := normalizeQueryList(args)
			if len(queryList) == 0 {
				return map[string]any{"error": "No query provided. Use 'query' or 'queries' parameter."}, nil
			}

			_, ok := cfg.ResolveProvider(getString(args, "provider"))
			if !ok {
				return map[string]any{"error": (&web.NoProviderError{}).Error()}, nil
			}

			opts := web.SearchOptions{
				NumResults:     getInt(args, "numResults", 5),
				RecencyFilter:  getString(args, "recencyFilter"),
				DomainFilter:   getStringSlice(args, "domainFilter"),
				IncludeContent: getBool(args, "includeContent"),
			}

			var allResults []web.QueryResultData
			var allURLs []string

			for _, query := range queryList {
				resp, err := web.Search(ctx, cfg, storage, query, opts)
				if err != nil {
					allResults = append(allResults, web.QueryResultData{
						Query: query,
						Error: err.Error(),
					})
					continue
				}

				allResults = append(allResults, web.QueryResultData{
					Query:    query,
					Answer:   resp.Answer,
					Results:  resp.Results,
					Provider: resp.Provider,
				})

				for _, r := range resp.Results {
					if !contains(allURLs, r.URL) {
						allURLs = append(allURLs, r.URL)
					}
				}
			}

			// Store results for later retrieval.
			id := storage.GenerateID()
			storage.Store(web.StoredData{
				ID:        id,
				Type:      "search",
				Timestamp: time.Now().Unix(),
				Queries:   allResults,
			})

			// Format output.
			output := formatSearchOutput(queryList, allResults, allURLs)
			return map[string]any{
				"output":    output,
				"searchId":  id,
				"queryCount": len(queryList),
				"totalResults": countResults(allResults),
			}, nil
		},
	}
}

func normalizeQueryList(args map[string]any) []string {
	var list []string

	// Try 'queries' array first.
	if raw, ok := args["queries"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					list = append(list, strings.TrimSpace(s))
				}
			}
		}
	}

	// Fall back to single 'query'.
	if len(list) == 0 {
		if q := getString(args, "query"); q != "" {
			list = append(list, q)
		}
	}

	return list
}

func formatSearchOutput(queryList []string, results []web.QueryResultData, urls []string) string {
	var b strings.Builder
	sc := 0
	tr := 0
	for _, r := range results {
		if r.Error == "" {
			sc++
			tr += len(r.Results)
		}
	}

	b.WriteString(fmt.Sprintf("Search: %d/%d queries succeeded, %d results\n\n", sc, len(queryList), tr))

	for _, r := range results {
		if len(queryList) > 1 {
			provider := ""
			if r.Provider != "" && r.Provider != "auto" {
				provider = fmt.Sprintf(" (%s)", r.Provider)
			}
			b.WriteString(fmt.Sprintf("## Query: \"%s\"%s\n\n", r.Query, provider))
		}

		if r.Error != "" {
			b.WriteString(fmt.Sprintf("Error: %s\n\n", r.Error))
			continue
		}

		if r.Answer != "" {
			b.WriteString(r.Answer)
			b.WriteString("\n\n---\n\n")
		}

		if len(r.Results) == 0 {
			b.WriteString("No results found.\n\n")
			continue
		}

		b.WriteString("**Sources:**\n")
		for i, src := range r.Results {
			title := src.Title
			if title == "" {
				title = src.URL
			}
			b.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, title, src.URL))
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func countResults(results []web.QueryResultData) int {
	var n int
	for _, r := range results {
		n += len(r.Results)
	}
	return n
}

// --- helpers ---

func getString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func getInt(args map[string]any, key string, fallback int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return fallback
}

func getBool(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getStringSlice(args map[string]any, key string) []string {
	if v, ok := args[key]; ok {
		if arr, ok := v.([]any); ok {
			var out []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
