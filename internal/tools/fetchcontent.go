package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"crobot/internal/web"
)

// FetchContentTool creates the web_fetch tool.
func FetchContentTool(cfg *web.Config, storage *web.Storage) Tool {
	return Tool{
		Name:        "web_fetch",
		Description: "Fetch URL(s) and extract readable content as markdown. Supports YouTube video transcripts (with thumbnail), GitHub repository contents, and local video files (with frame thumbnail). Uses Gemini for video analysis. Falls back to Jina Reader when pages block bots.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Single URL to fetch",
				},
				"urls": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Multiple URLs (parallel)",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Question or instruction for video analysis (YouTube and local video files). Pass the user's specific question here.",
				},
				"timestamp": map[string]any{
					"type":        "string",
					"description": "Extract video frame(s) at a timestamp or time range. Single: '1:23:45', '23:45', or '85' (seconds). Range: '23:41-25:00'.",
				},
				"frames": map[string]any{
					"type":        "number",
					"description": "Number of frames to extract (max 12)",
				},
				"forceClone": map[string]any{
					"type":        "boolean",
					"description": "Force cloning large GitHub repositories that exceed the size threshold",
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			urls := normalizeURLList(args)
			if len(urls) == 0 {
				return map[string]any{"error": "No URL provided. Use 'url' or 'urls' parameter."}, nil
			}

			extractOpts := web.ExtractOptions{
				Prompt:     getString(args, "prompt"),
				Timestamp:  getString(args, "timestamp"),
				Frames:     getInt(args, "frames", 0),
				ForceClone: getBool(args, "forceClone"),
			}

			var contents []web.ExtractedContent
			for _, url := range urls {
				content, err := web.ExtractContent(ctx, url, cfg, extractOpts)
				if err != nil {
					contents = append(contents, web.ExtractedContent{
						URL:   url,
						Error: err.Error(),
					})
					continue
				}
				contents = append(contents, *content)
			}

			// Store for later retrieval.
			id := storage.GenerateID()
			storage.Store(web.StoredData{
				ID:        id,
				Type:      "fetch",
				Timestamp: time.Now().Unix(),
				URLs:      contents,
			})

			output := formatFetchOutput(urls, contents, id)
			return map[string]any{
				"output":  output,
				"fetchId": id,
				"urlCount": len(urls),
			}, nil
		},
	}
}

func normalizeURLList(args map[string]any) []string {
	var list []string

	if raw, ok := args["urls"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					list = append(list, strings.TrimSpace(s))
				}
			}
		}
	}

	if len(list) == 0 {
		if u := getString(args, "url"); u != "" {
			list = append(list, u)
		}
	}

	return list
}

func formatFetchOutput(urls []string, contents []web.ExtractedContent, id string) string {
	var b strings.Builder
	ok := 0
	for _, c := range contents {
		if c.Error == "" {
			ok++
		}
	}

	b.WriteString(fmt.Sprintf("## Fetched Content\n\n"))
	b.WriteString(fmt.Sprintf("**%d/%d URL(s) retrieved** (`fetchId: %s`)\n\n", ok, len(urls), id))

	for i, c := range contents {
		label := c.URL
		if c.Title != "" {
			label = fmt.Sprintf("%s — %s", c.Title, c.URL)
		}

		b.WriteString(fmt.Sprintf("---\n\n### %d. %s\n\n", i+1, label))

		if c.Error != "" {
			b.WriteString(fmt.Sprintf("> Error: %s\n\n", c.Error))
			continue
		}

		content := c.Content
		maxLen := maxInlineContent
		if len(content) > maxLen {
			content = content[:maxLen]
			b.WriteString(content)
			b.WriteString(fmt.Sprintf("\n\n> Content truncated at %d characters. Use `get_search_content` with `fetchId: %s` and `url: %q` for full content.\n\n", maxLen, id, c.URL))
		} else {
			b.WriteString(content)
			b.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(b.String())
}
