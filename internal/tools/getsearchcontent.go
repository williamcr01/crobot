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
		Description: "Retrieve full content from a previous web_search or fetch_content call. Content over 30,000 chars is truncated in tool responses but stored in full for retrieval here.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"responseId": map[string]any{
					"type":        "string",
					"description": "The responseId from web_search or fetch_content",
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
	var queries []map[string]any
	for _, q := range data.Queries {
		var results []map[string]any
		for _, r := range q.Results {
			results = append(results, map[string]any{
				"title":   r.Title,
				"url":     r.URL,
				"snippet": r.Snippet,
			})
		}
		queries = append(queries, map[string]any{
			"query":    q.Query,
			"answer":   q.Answer,
			"results":  results,
			"error":    q.Error,
			"provider": q.Provider,
		})
	}

	return map[string]any{
		"id":        data.ID,
		"type":      "search",
		"timestamp": data.Timestamp,
		"queries":   queries,
	}
}

func formatFetchResult(data web.StoredData) map[string]any {
	var urls []map[string]any
	for _, u := range data.URLs {
		entry := map[string]any{
			"url":   u.URL,
			"title": u.Title,
		}
		if u.Error != "" {
			entry["error"] = u.Error
		} else {
			entry["content"] = u.Content
		}
		urls = append(urls, entry)
	}

	var summary strings.Builder
	for _, u := range data.URLs {
		if u.Error != "" {
			summary.WriteString(fmt.Sprintf("%s: error (%s)\n", u.URL, u.Error))
		} else {
			summary.WriteString(fmt.Sprintf("%s: %d chars\n", u.URL, len(u.Content)))
		}
	}

	return map[string]any{
		"id":        data.ID,
		"type":      "fetch",
		"timestamp": data.Timestamp,
		"urls":      urls,
		"summary":   strings.TrimSpace(summary.String()),
	}
}
