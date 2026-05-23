package scanner

import "strings"

// Scanner holds loaded secrets and performs redaction.
type Scanner struct {
	secrets map[string]string // key name -> secret value
}

// New creates a Scanner loaded with the given secrets map.
func New(secrets map[string]string) *Scanner {
	return &Scanner{secrets: secrets}
}

// Redact scans body for any known secret values and replaces them with
// [REDACTED:<KEY_NAME>]. Returns the (possibly modified) body and the list
// of key names that were redacted.
func (s *Scanner) Redact(body string) (string, []string) {
	var redactedKeys []string
	result := body

	for key, val := range s.secrets {
		if strings.Contains(result, val) {
			placeholder := "[REDACTED:" + key + "]"
			result = strings.ReplaceAll(result, val, placeholder)
			redactedKeys = append(redactedKeys, key)
		}
	}

	return result, redactedKeys
}
