package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

var FileWriteTool = Tool{
	Name:        "file_write",
	Description: "Write or create a file. Auto-creates parent directories.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		oldContent := ""
		created := false
		if data, err := os.ReadFile(path); err == nil {
			oldContent = string(data)
		} else if os.IsNotExist(err) {
			created = true
		} else {
			return nil, fmt.Errorf("read existing file: %w", err)
		}

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create directory: %w", err)
		}

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}

		diff := unifiedDiff(path, oldContent, content)
		return map[string]any{
			"path":          path,
			"written":       len(content),
			"created":       created,
			"diff":          diff.Text,
			"diffTruncated": diff.Truncated,
			"success":       true,
		}, nil
	},
}
