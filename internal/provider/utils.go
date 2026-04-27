package provider

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ParseJSON unmarshals a JSON string into v.
func ParseJSON(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}

// IDNumber returns a numeric ID from the call ID string.
// OpenRouter call IDs are strings like "call_abc123". This returns 0 as a fallback.
func (t ToolCall) IDNumber() int64 {
	parts := strings.SplitN(t.ID, "_", 2)
	if len(parts) == 2 {
		if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return n
		}
	}
	return 0
}