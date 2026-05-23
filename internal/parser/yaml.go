package parser

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseYAMLFile parses a YAML file and returns flattened key=value pairs.
// Nested keys are dot-separated. Only string leaf values are included.
func ParseYAMLFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	flattenYAML("", raw, result)
	return result, nil
}

func flattenYAML(prefix string, v interface{}, out map[string]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenYAML(key, child, out)
		}
	case map[interface{}]interface{}:
		for k, child := range val {
			ks, _ := k.(string)
			key := ks
			if prefix != "" {
				key = prefix + "." + ks
			}
			flattenYAML(key, child, out)
		}
	case []interface{}:
		for i, child := range val {
			key := fmt.Sprintf("%s.%d", prefix, i)
			if prefix == "" {
				key = fmt.Sprintf("%d", i)
			}
			flattenYAML(key, child, out)
		}
	case string:
		if prefix != "" {
			out[prefix] = val
		}
	}
}
