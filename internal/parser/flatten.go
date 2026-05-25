package parser

import "fmt"

// flatten recursively walks a parsed JSON/YAML value and writes all string
// leaf nodes into out with dot-separated keys.
// allowIfaceMap enables handling of map[interface{}]interface{} which
// gopkg.in/yaml.v3 can produce for non-string-keyed YAML maps.
func flatten(prefix string, v interface{}, out map[string]string, allowIfaceMap bool) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, child, out, allowIfaceMap)
		}
	case map[interface{}]interface{}:
		if !allowIfaceMap {
			return
		}
		for k, child := range val {
			ks, _ := k.(string)
			key := ks
			if prefix != "" {
				key = prefix + "." + ks
			}
			flatten(key, child, out, allowIfaceMap)
		}
	case []interface{}:
		for i, child := range val {
			key := fmt.Sprintf("%s.%d", prefix, i)
			if prefix == "" {
				key = fmt.Sprintf("%d", i)
			}
			flatten(key, child, out, allowIfaceMap)
		}
	case string:
		if prefix != "" {
			out[prefix] = val
		}
	}
}
