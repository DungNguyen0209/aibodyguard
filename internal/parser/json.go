package parser

import (
	"encoding/json"
	"os"
)

// ParseJSONFile parses a JSON file and returns flattened key=value pairs.
// Nested keys are dot-separated (e.g., "database.password").
// Only string leaf values are included.
func ParseJSONFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	flatten("", raw, result, false)
	return result, nil
}
