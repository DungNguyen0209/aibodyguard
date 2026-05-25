package parser

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAMLFile parses a YAML file and returns flattened key=value pairs.
// Nested keys are dot-separated. Only string leaf values are included.
// Commented-out lines that contain valid YAML key: value pairs are also
// parsed — a commented-out credential is still a credential.
func ParseYAMLFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)

	// Pass 1: parse the active (non-commented) YAML normally.
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	flattenYAML("", raw, result)

	// Pass 2: extract commented-out lines and attempt to parse each as a
	// standalone YAML snippet. Indentation is stripped so that nested
	// commented keys like "#  password: secret" parse as top-level keys.
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Strip leading # characters and surrounding whitespace.
		candidate := strings.TrimLeft(trimmed, "#")
		candidate = strings.TrimSpace(candidate)

		// Must look like "key: value" — skip plain prose comments.
		colonIdx := strings.Index(candidate, ":")
		if colonIdx < 0 {
			continue
		}

		// Try to unmarshal the candidate as a mini YAML document.
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
