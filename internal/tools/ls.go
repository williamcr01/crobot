package tools

import (
	"context"
	"fmt"
	"os"
	"sort"
)

const defaultLsLimit = 500

var LsTool = Tool{
	Name:        "ls",
	Description: "List directory contents. Returns entries sorted alphabetically, with '/' appended to directory names.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to list (default: current working directory)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of entries to return (default: 500)",
			},
		},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		path, _ := args["path"].(string)
		if path == "" {
			var err error
			path, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getwd: %w", err)
			}
		}

		limit := defaultLsLimit
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		if limit < 1 {
			limit = 1
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"error": fmt.Sprintf("path not found: %s", path)}, nil
			}
			if os.IsPermission(err) {
				return map[string]any{"error": fmt.Sprintf("permission denied: %s", path)}, nil
			}
			// Check if it's a file, not a directory.
			if fi, statErr := os.Stat(path); statErr == nil && !fi.IsDir() {
				return map[string]any{
					"error": fmt.Sprintf("not a directory: %s", path),
				}, nil
			}
			return nil, fmt.Errorf("read dir %s: %w", path, err)
		}

		result := make([]string, 0, len(entries))
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			result = append(result, name)
		}
		sort.Strings(result)

		truncated := false
		if len(result) > limit {
			result = result[:limit]
			truncated = true
		}

		out := map[string]any{
			"path":    path,
			"entries": result,
		}
		if truncated {
			out["truncated"] = true
		}

		return out, nil
	},
}
