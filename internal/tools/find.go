package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const defaultFindLimit = 1000

var FindTool = Tool{
	Name:        "find",
	Description: "Search for files by glob pattern. Respects .gitignore.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files, e.g. '*.go', '**/*_test.go'",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in (default: current working directory)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 1000)",
			},
		},
		"required": []string{"pattern"},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return nil, fmt.Errorf("pattern is required")
		}

		root, _ := args["path"].(string)
		if root == "" {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getwd: %w", err)
			}
		}

		root, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("abs path: %w", err)
		}

		// Verify root exists and is a directory.
		rootInfo, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"error": fmt.Sprintf("path not found: %s", root)}, nil
			}
			return nil, fmt.Errorf("stat %s: %w", root, err)
		}
		if !rootInfo.IsDir() {
			return map[string]any{"error": fmt.Sprintf("not a directory: %s", root)}, nil
		}

		limit := defaultFindLimit
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		if limit < 1 {
			limit = 1
		}

		// Load gitignore matcher from root.
		ignoreMatcher, err := NewIgnoreMatcher(root)
		if err != nil {
			return nil, fmt.Errorf("gitignore: %w", err)
		}

		var results []string

		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				// Skip directories we can't read.
				return filepath.SkipDir
			}

			// Get relative path for matching and gitignore check.
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}

			// Skip ignored paths and directories.
			if ignoreMatcher.ShouldIgnore(relPath, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Only match files, not directories.
			if d.IsDir() {
				// Ensure rules are loaded for this subdirectory before walking into it.
				ignoreMatcher.EnsureRulesLoaded(relPath)
				return nil
			}

			// Test the pattern. If pattern contains a /, match against relPath.
			// Otherwise, match against the base name.
			var match bool
			if containsSlash(pattern) {
				match, _ = filepath.Match(pattern, relPath)
			} else {
				match, _ = filepath.Match(pattern, d.Name())
			}

			if match {
				results = append(results, relPath)
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", root, err)
		}

		sort.Strings(results)

		truncated := false
		if len(results) > limit {
			results = results[:limit]
			truncated = true
		}

		out := map[string]any{
			"files":   results,
			"pattern": pattern,
		}
		if truncated {
			out["truncated"] = true
		}

		return out, nil
	},
}

// containsSlash reports whether s contains a forward or back slash.
func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '\\' {
			return true
		}
	}
	return false
}
