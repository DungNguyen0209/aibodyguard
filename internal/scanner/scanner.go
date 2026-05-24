package scanner

import (
	"sort"
	"strings"
)

// Scanner redacts known secret values from arbitrary text.
type Scanner interface {
	Redact(input string) (cleaned string, redactedValues []string)
}

// New returns a Scanner loaded with the given secrets map.
// All distinct values across all keys are flattened into a hash set for O(1) dedup.
func New(secrets map[string][]string) Scanner {
	seen := make(map[string]struct{})
	for _, vals := range secrets {
		for _, v := range vals {
			if v != "" {
				seen[v] = struct{}{}
			}
		}
	}
	return &redactScanner{values: seen}
}

// redactScanner is the concrete implementation of Scanner.
type redactScanner struct {
	values map[string]struct{} // hash set of all secret values
}

// Redact scans body for any known secret values and replaces them with ****.
// Values are matched longest-first to prevent substring collisions.
// Returns the (possibly modified) body and the sorted list of matched secret values.
func (s *redactScanner) Redact(body string) (string, []string) {
	// Build sorted slice from hash set — longest value first
	vals := make([]string, 0, len(s.values))
	for v := range s.values {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		return len(vals[i]) > len(vals[j])
	})

	var matched []string
	result := body

	for _, v := range vals {
		if strings.Contains(result, v) {
			result = strings.ReplaceAll(result, v, "****")
			matched = append(matched, v)
		}
	}

	sort.Strings(matched)
	return result, matched
}
