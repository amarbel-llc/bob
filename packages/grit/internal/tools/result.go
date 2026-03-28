package tools

import "encoding/json"

// coerceStringToArray recovers from a common LLM mistake: passing a JSON array
// as a string value (e.g. {"paths": "[\"a\",\"b\"]"} instead of {"paths": ["a","b"]}).
// It extracts the named field as a raw string, then attempts to parse it as a
// JSON array of strings. Returns nil if the field is not a string or not valid JSON.
func coerceStringToArray(args json.RawMessage, field string) []string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil
	}

	val, ok := raw[field]
	if !ok {
		return nil
	}

	// Check if the value is a JSON string (starts with `"`).
	var s string
	if err := json.Unmarshal(val, &s); err != nil {
		return nil
	}

	// Try to parse the string contents as a JSON array.
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		// Not a JSON array — treat the whole string as a single path.
		if s != "" {
			return []string{s}
		}
		return nil
	}

	return result
}
