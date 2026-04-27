package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

var FileEditTool = Tool{
	Name:        "file_edit",
	Description: "Edit a file by replacing an exact text match. Supports single replacement only.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file",
			},
			"oldText": map[string]any{
				"type":        "string",
				"description": "Exact text to find and replace",
			},
			"newText": map[string]any{
				"type":        "string",
				"description": "Replacement text",
			},
		},
		"required": []string{"path", "oldText", "newText"},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		path, _ := args["path"].(string)
		oldText, _ := args["oldText"].(string)
		newText, _ := args["newText"].(string)

		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if oldText == "" {
			return nil, fmt.Errorf("oldText is required")
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"error": fmt.Sprintf("File not found: %s", path)}, nil
			}
			return nil, fmt.Errorf("read file: %w", err)
		}

		content := string(data)
		count := strings.Count(content, oldText)
		if count == 0 {
			return map[string]any{"error": "oldText not found in file"}, nil
		}
		if count > 1 {
			return map[string]any{
				"error": fmt.Sprintf("oldText matches %d occurrences; must be unique. Provide more context.", count),
			}, nil
		}

		newContent := strings.Replace(content, oldText, newText, 1)
		if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}

		// Calculate the line number of the change.
		idx := strings.Index(content, oldText)
		line := strings.Count(content[:idx], "\n") + 1

		return map[string]any{
			"path":    path,
			"line":    line,
			"success": true,
		}, nil
	},
}
