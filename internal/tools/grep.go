package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	defaultGrepLimit = 100
	maxLineLength    = 500
	binaryCheckBytes = 8192
)

var GrepTool = Tool{
	Name:        "grep",
	Description: "Search file contents for a pattern. Respects .gitignore. Returns matching file paths with line numbers and content.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex or literal search pattern",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search (default: current working directory)",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Only search files matching this glob pattern, e.g. '*.go' or '**/*.ts'",
			},
			"ignoreCase": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive search (default: false)",
			},
			"literal": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as a literal string instead of regex (default: false)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of matches to return (default: 100)",
			},
		},
		"required": []string{"pattern"},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return nil, fmt.Errorf("pattern is required")
		}

		path, _ := args["path"].(string)
		if path == "" {
			var err error
			path, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getwd: %w", err)
			}
		}

		globFilter, _ := args["glob"].(string)
		ignoreCase, _ := args["ignoreCase"].(bool)
		literalSearch, _ := args["literal"].(bool)

		limit := defaultGrepLimit
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		if limit < 1 {
			limit = 1
		}

		// Compile the regex.
		rePattern := pattern
		if literalSearch {
			rePattern = regexp.QuoteMeta(pattern)
		}
		if ignoreCase {
			rePattern = "(?i)" + rePattern
		}

		re, err := regexp.Compile(rePattern)
		if err != nil {
			return map[string]any{
				"error": fmt.Sprintf("invalid pattern: %s", err.Error()),
			}, nil
		}

		path, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("abs path: %w", err)
		}

		// Stat the path to see if it's a file or directory.
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"error": fmt.Sprintf("path not found: %s", path)}, nil
			}
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		// If it's a file, search just that file.
		if !info.IsDir() {
			results, err := searchFile(path, re, limit)
			if err != nil {
				return nil, fmt.Errorf("search file: %w", err)
			}
			out := map[string]any{
				"matches": results,
			}
			if len(results) >= limit {
				out["truncated"] = true
			}
			return out, nil
		}

		// It's a directory — walk and search.
		root := path

		// Load gitignore matcher.
		ignoreMatcher, err := NewIgnoreMatcher(root)
		if err != nil {
			return nil, fmt.Errorf("gitignore: %w", err)
		}

		var allMatches []matchResult

		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}

			// Get relative path for gitignore check.
			relPath, _ := filepath.Rel(root, path)

			// Skip gitignored paths.
			if ignoreMatcher.ShouldIgnore(relPath, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip directories.
			if d.IsDir() {
				ignoreMatcher.EnsureRulesLoaded(relPath)
				return nil
			}

			// Skip symlinks.
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}

			// Apply glob filter if set.
			if globFilter != "" {
				match, _ := filepath.Match(globFilter, d.Name())
				if !match {
					return nil
				}
			}

			// Check for binary file.
			if isBinaryFile(path) {
				return nil
			}

			// Search this file.
			results, searchErr := searchFile(path, re, limit-len(allMatches))
			if searchErr != nil {
				// Skip files that can't be read.
				return nil
			}

			// Map the file path to relative for each match.
			for i := range results {
				results[i].File = relPath
			}

			allMatches = append(allMatches, results...)

			// Stop if we hit the limit.
			if len(allMatches) >= limit {
				return filepath.SkipAll
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", root, err)
		}

		// Sort by file path, then line number.
		sort.Slice(allMatches, func(i, j int) bool {
			if allMatches[i].File != allMatches[j].File {
				return allMatches[i].File < allMatches[j].File
			}
			return allMatches[i].Line < allMatches[j].Line
		})

		truncated := len(allMatches) >= limit
		if len(allMatches) > limit {
			allMatches = allMatches[:limit]
		}

		out := map[string]any{
			"matches": allMatches,
		}
		if truncated {
			out["truncated"] = true
		}

		return out, nil
	},
}

type matchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// searchFile searches a single file with the compiled regex.
// Returns up to limit matches. File paths are absolute.
func searchFile(path string, re *regexp.Regexp, limit int) ([]matchResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var results []matchResult

	for i, line := range lines {
		if len(results) >= limit {
			break
		}
		if re.MatchString(line) {
			content := truncateLine(line, maxLineLength)
			results = append(results, matchResult{
				File:    path,
				Line:    i + 1, // 1-indexed
				Content: content,
			})
		}
	}

	return results, nil
}

// truncateLine truncates a line to maxLen, preserving UTF-8 boundaries.
func truncateLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	trunc := s[:maxLen]
	for len(trunc) > 0 && !utf8.ValidString(trunc) {
		trunc = trunc[:len(trunc)-1]
	}
	return trunc + "..."
}

// isBinaryFile checks if a file is likely binary by looking for null bytes
// in the first binaryCheckBytes.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true // can't read, treat as binary
	}
	defer f.Close()

	buf := make([]byte, binaryCheckBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return true
	}
	buf = buf[:n]

	for _, b := range buf {
		if b == 0 {
			return true
		}
	}
	return false
}
