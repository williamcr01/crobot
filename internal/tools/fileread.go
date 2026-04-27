package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func init() {
	// Registered during config setup. Definition kept here for clarity.
}

var FileReadTool = Tool{
	Name:        "file_read",
	Description: "Read the contents of a file at the given path",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Start reading from this line (1-indexed)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to return",
			},
		},
		"required": []string{"path"},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		path, _ := args["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"error": fmt.Sprintf("File not found: %s", path)}, nil
			}
			if os.IsPermission(err) {
				return map[string]any{"error": fmt.Sprintf("Permission denied: %s", path)}, nil
			}
			return nil, err
		}

		// Detect images by magic bytes.
		if isImage(data) {
			mime := detectMIME(data)
			b64 := base64.StdEncoding.EncodeToString(data)
			return map[string]any{
				"type":     "image",
				"mimeType": mime,
				"data":     b64,
			}, nil
		}

		lines := strings.Split(string(data), "\n")
		offset := 0
		if o, ok := args["offset"].(float64); ok {
			offset = int(o) - 1
			if offset < 0 {
				offset = 0
			}
		}
		limit := len(lines) - offset
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		end := offset + limit
		if end > len(lines) {
			end = len(lines)
		}
		slice := lines[offset:end]

		result := map[string]any{
			"content":    strings.Join(slice, "\n"),
			"totalLines": len(lines),
		}
		if end < len(lines) {
			result["truncated"] = true
			result["nextOffset"] = end + 1
		}
		return result, nil
	},
}

// isImage checks magic bytes for common image formats.
func isImage(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	// PNG
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return true
	}
	// JPEG
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}
	// GIF
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return true
	}
	// WebP
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return true
	}
	return false
}

// detectMIME returns the MIME type from magic bytes.
func detectMIME(data []byte) string {
	if len(data) < 12 {
		return "application/octet-stream"
	}
	switch {
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return "image/png"
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38:
		return "image/gif"
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50:
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

