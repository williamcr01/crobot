package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
	"unicode/utf8"
)

var BashTool = Tool{
	Name:        "bash",
	Description: "Execute a bash command with timeout and output capture",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Bash command to execute",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default: 30, max: 120)",
			},
		},
		"required": []string{"command"},
	},
	Execute: func(ctx context.Context, args map[string]any) (any, error) {
		command, _ := args["command"].(string)
		if command == "" {
			return nil, fmt.Errorf("command is required")
		}

		timeout := 30
		if t, ok := args["timeout"].(float64); ok {
			timeout = int(t)
		}
		if timeout > 120 {
			timeout = 120
		}
		if timeout < 1 {
			timeout = 1
		}

		cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, "bash", "-lc", command)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				return map[string]any{
					"stdout":   truncateOutput(stdout.String(), 10000),
					"stderr":   truncateOutput(stderr.String(), 10000),
					"exitCode": -1,
					"error":    err.Error(),
				}, nil
			}
		}

		stdoutStr := truncateOutput(stdout.String(), 10000)
		stderrStr := truncateOutput(stderr.String(), 10000)

		result := map[string]any{
			"stdout":   stdoutStr,
			"stderr":   stderrStr,
			"exitCode": exitCode,
		}

		if len(stdoutStr) >= 10000 || len(stderrStr) >= 10000 {
			result["truncated"] = true
		}

		return result, nil
	},
}

// truncateOutput safely truncates a string to maxLen bytes,
// preserving valid UTF-8 boundaries.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find the last valid UTF-8 boundary before maxLen.
	trunc := s[:maxLen]
	for len(trunc) > 0 && !utf8.ValidString(trunc) {
		trunc = trunc[:len(trunc)-1]
	}
	return trunc + "... (truncated)"
}
