package tools

import (
	"context"
	"fmt"
	"strings"

	"crobot/internal/web"
)

// GetSearchContentTool creates the get_search_content tool for retrieving stored
// search and fetch results.
func GetSearchContentTool(storage *web.Storage) Tool {
	return Tool{
		Name:        "get_search_content",
		Description: "Retrieve full content from a previous web_search or web_fetch call. Content over 30,000 chars is truncated in initial tool responses but stored in full for retrieval here.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"responseId": map[string]any{
					"type":        "string",
					"description": "The responseId from web_search or web_fetch",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Get content for this query (web_search)",
				},
				"queryIndex": map[string]any{
					"type":        "number",
					"description": "Get content for query at index",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "Get content for this URL",
				},
				"urlIndex": map[string]any{
					"type":        "number",
					"description": "Get content for URL at index",
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			// Try direct ID lookup first.
			if id := getString(args, "responseId"); id != "" {
				data, ok := storage.Get(id)
				if !ok {
					return map[string]any{
						"error": fmt.Sprintf("No stored result with id %q", id),
					}, nil
				}
				return formatStoredResult(data), nil
			}

			// Try query-based lookup.
			if q := getString(args, "query"); q != "" {
				results := storage.FindByQuery(q)
				if len(results) == 0 {
					return map[string]any{
						"error": fmt.Sprintf("No stored results for query %q", q),
					}, nil
				}
				qi := getInt(args, "queryIndex", 0)
				if qi < 0 || qi >= len(results) {
					return map[string]any{
						"error": fmt.Sprintf("Query index %d out of range (0-%d)", qi, len(results)-1),
					}, nil
				}
				return formatStoredResult(results[qi]), nil
			}

			// Try URL-based lookup.
			if u := getString(args, "url"); u != "" {
				results := storage.FindByURL(u)
				if len(results) == 0 {
					return map[string]any{
						"error": fmt.Sprintf("No stored results for URL %q", u),
					}, nil
				}
				ui := getInt(args, "urlIndex", 0)
				if ui < 0 || ui >= len(results) {
					return map[string]any{
						"error": fmt.Sprintf("URL index %d out of range (0-%d)", ui, len(results)-1),
					}, nil
				}
				return formatStoredResult(results[ui]), nil
			}

			return map[string]any{
				"error": "No lookup key provided. Use 'responseId', 'query', or 'url'.",
			}, nil
		},
	}
}

func formatStoredResult(data web.StoredData) map[string]any {
	switch data.Type {
	case "search":
		return formatSearchResult(data)
	case "fetch":
		return formatFetchResult(data)
	default:
		return map[string]any{
			"error": fmt.Sprintf("Unknown stored data type: %q", data.Type),
		}
	}
}

func formatSearchResult(data web.StoredData) map[string]any {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Stored Search Results\n\n"))
	b.WriteString(fmt.Sprintf("**ID:** `%s`  \n", data.ID))
	b.WriteString(fmt.Sprintf("**Type:** search  \n"))
	b.WriteString(fmt.Sprintf("**Queries:** %d\n\n", len(data.Queries)))

	totalResults := 0
	for _, q := range data.Queries {
		totalResults += len(q.Results)
	}

	for i, q := range data.Queries {
		providerInfo := ""
		if q.Provider != "" && q.Provider != "auto" {
			providerInfo = fmt.Sprintf(" `[%s]`", q.Provider)
		}
		b.WriteString(fmt.Sprintf("---\n\n### %d. `%s`%s\n\n", i+1, q.Query, providerInfo))

		if q.Error != "" {
			b.WriteString(fmt.Sprintf("> Error: %s\n\n", q.Error))
			continue
		}

		if q.Answer != "" {
			b.WriteString(q.Answer)
			b.WriteString("\n\n")
		}

		if len(q.Results) > 0 {
			b.WriteString(fmt.Sprintf("**Sources (%d):**\n\n", len(q.Results)))
			for j, r := range q.Results {
				title := r.Title
				if title == "" {
					title = r.URL
				}
				b.WriteString(fmt.Sprintf("%d. [%s](%s)", j+1, title, r.URL))
				if r.Snippet != "" {
					snippet := r.Snippet
					if len(snippet) > 200 {
						snippet = snippet[:200] + "..."
					}
					b.WriteString(fmt.Sprintf("\n   > %s", snippet))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		if len(q.Results) == 0 && q.Error == "" {
			b.WriteString("_No results._\n\n")
		}
	}

	return map[string]any{
		"output":   strings.TrimSpace(b.String()),
		"id":       data.ID,
		"type":     "search",
		"queries":  len(data.Queries),
		"results":  totalResults,
	}
}

func formatFetchResult(data web.StoredData) map[string]any {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Stored Fetched Content\n\n"))
	b.WriteString(fmt.Sprintf("**ID:** `%s`  \n", data.ID))
	b.WriteString(fmt.Sprintf("**Type:** fetch  \n"))
	b.WriteString(fmt.Sprintf("**URLs:** %d\n\n", len(data.URLs)))

	for i, u := range data.URLs {
		label := u.URL
		if u.Title != "" {
			label = fmt.Sprintf("%s — %s", u.Title, u.URL)
		}

		b.WriteString(fmt.Sprintf("---\n\n### %d. %s\n\n", i+1, label))

		if u.Error != "" {
			b.WriteString(fmt.Sprintf("> Error: %s\n\n", u.Error))
			continue
		}

		b.WriteString(fmt.Sprintf("**Length:** %d characters\n\n", len(u.Content)))

		if len(u.Content) > 0 {
			// Show first 2000 chars as preview.
			preview := u.Content
			if len(preview) > 2000 {
				preview = preview[:2000]
				b.WriteString(preview)
				b.WriteString(fmt.Sprintf("\n\n_First 2000 of %d characters shown._\n\n", len(u.Content)))
			} else {
				b.WriteString(preview)
				b.WriteString("\n\n")
			}
		}
	}

	return map[string]any{
		"output":   strings.TrimSpace(b.String()),
		"id":       data.ID,
		"type":     "fetch",
		"urls":     len(data.URLs),
	}
}
