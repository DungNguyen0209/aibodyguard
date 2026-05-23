package parser

import (
	"encoding/json"
	"fmt"
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
	flattenJSON("", raw, result)
	return result, nil
}

func flattenJSON(prefix string, v interface{}, out map[string]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenJSON(key, child, out)
		}
	case []interface{}:
		for i, child := range val {
			key := fmt.Sprintf("%s.%d", prefix, i)
			if prefix == "" {
				key = fmt.Sprintf("%d", i)
			}
			flattenJSON(key, child, out)
		}
	case string:
		if prefix != "" {
			out[prefix] = val
		}
	}
}
