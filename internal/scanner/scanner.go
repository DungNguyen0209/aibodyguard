package scanner

import (
	"sort"
	"strings"
)

// Scanner redacts known secret values from arbitrary text.
type Scanner interface {
	Redact(input string) (cleaned string, redactedKeys []string)
}

// New returns a Scanner loaded with the given secrets map.
func New(secrets map[string]string) Scanner {
	return &redactScanner{secrets: secrets}
}

// redactScanner is the concrete implementation of Scanner.
type redactScanner struct {
	secrets map[string]string
}

// entry is a key-value pair used for sorted processing.
type entry struct {
	key string
	val string
}

// Redact scans body for any known secret values and replaces them with
// [REDACTED:<KEY_NAME>]. Secrets are matched longest-value-first to prevent
// substring collisions. Returns the (possibly modified) body and the sorted
// list of redacted key names.
func (s *redactScanner) Redact(body string) (string, []string) {
	entries := make([]entry, 0, len(s.secrets))
	for k, v := range s.secrets {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].val) > len(entries[j].val)
	})

	var redactedKeys []string
	result := body

	for _, e := range entries {
		if strings.Contains(result, e.val) {
			placeholder := "[REDACTED:" + e.key + "]"
			result = strings.ReplaceAll(result, e.val, placeholder)
			redactedKeys = append(redactedKeys, e.key)
		}
	}

	sort.Strings(redactedKeys)
	return result, redactedKeys
}
