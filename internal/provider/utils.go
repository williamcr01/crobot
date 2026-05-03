package provider

import (
	"encoding/json"
)

// ParseJSON unmarshals a JSON string into v.
func ParseJSON(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}

// metadataUserAgent returns a User-Agent string from request metadata.
// Falls back to "crobot" when no "user_agent" key is set.
func metadataUserAgent(metadata map[string]string) string {
	if metadata != nil {
		if ua, ok := metadata["user_agent"]; ok && ua != "" {
			return ua
		}
	}
	return "crobot"
}