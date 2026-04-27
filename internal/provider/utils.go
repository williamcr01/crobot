package provider

import "encoding/json"

// ParseJSON unmarshals a JSON string into v.
func ParseJSON(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}
