package parser

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAMLFile parses a YAML file and returns flattened key=value pairs.
// Only active (non-commented) content is parsed — the YAML library handles
// this naturally. Nested keys are dot-separated. Only string leaf values
// are included.
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

// ParseCommentedYAMLFile extracts commented-out key: value pairs from a YAML
// file. Each line beginning with # is stripped of its comment marker and the
// remainder is attempted as a standalone YAML snippet. Lines that do not parse
// as valid YAML key: value (plain prose comments) are silently skipped.
// This function never affects the result of ParseYAMLFile.
func ParseCommentedYAMLFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Strip leading # characters and surrounding whitespace.
		candidate := strings.TrimLeft(trimmed, "#")
		candidate = strings.TrimSpace(candidate)

		// Must contain a colon to be worth attempting — skips pure prose.
		if !strings.Contains(candidate, ":") {
			continue
		}

		// Try to unmarshal as a mini YAML document.
		var mini interface{}
		if err := yaml.Unmarshal([]byte(candidate), &mini); err != nil {
			continue // not valid YAML — skip silently
		}
		flattenYAML("", mini, result)
	}
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
