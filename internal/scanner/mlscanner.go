package scanner

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/DungNguyen0209/aibodyguard/internal/detector"
)

// secretDetector is satisfied by *detector.Detector and can be faked in tests.
type secretDetector interface {
	Available() bool
	DetectFromContent(content string) ([]string, error)
}

// MLScanner extends the static secret scanner with runtime ML detection.
// It wraps a static set of known secrets (from file scanning) plus a
// dynamically-grown set discovered at runtime via the distilbert model.
// Implements the Scanner interface — drop-in replacement.
type MLScanner struct {
	static  map[string]struct{}
	dynamic map[string]struct{}
	det     secretDetector
	mu      sync.RWMutex
	log     io.Writer
}

// NewMLScanner returns an MLScanner that redacts known secrets and
// uses the detector (if available) to discover new ones at runtime.
func NewMLScanner(secrets map[string][]string, det *detector.Detector, log io.Writer) *MLScanner {
	static := make(map[string]struct{})
	for _, vals := range secrets {
		for _, v := range vals {
			if v != "" {
				static[v] = struct{}{}
			}
		}
	}
	return &MLScanner{
		static:  static,
		dynamic: make(map[string]struct{}),
		det:     det,
		log:     log,
	}
}

// Redact redacts known secrets and discovers new ones via ML.
// Newly discovered secrets are added to the dynamic store so all
// subsequent requests redact them by string match.
// For JSON bodies, secrets are only replaced within string values
// to avoid corrupting JSON structure.
// NOTE: ML detection on request bodies is disabled — the distilbert
// model is trained on credential files and produces too many false
// positives on chat traffic. Only file-sourced secrets are used.
func (s *MLScanner) Redact(input string) (string, []string) {
	s.mu.RLock()
	total := len(s.static) + len(s.dynamic)
	vals := make([]string, 0, total)
	for v := range s.static {
		vals = append(vals, v)
	}
	for v := range s.dynamic {
		vals = append(vals, v)
	}
	s.mu.RUnlock()

	sort.Slice(vals, func(i, j int) bool {
		return len(vals[i]) > len(vals[j])
	})

	var matched []string
	result := s.redactString(input, vals, &matched)

	sort.Strings(matched)
	return result, matched
}

// redactString applies secret replacement to input text.
// If input is valid JSON, replacement only touches string values
// (never keys, numbers, booleans, or structural elements).
// For non-JSON text, naive string replacement is used.
func (s *MLScanner) redactString(input string, vals []string, matched *[]string) string {
	seen := make(map[string]struct{})

	var parsed interface{}
	if err := json.Unmarshal([]byte(input), &parsed); err == nil {
		changed := s.redactJSONValue(&parsed, vals, seen)
		if changed {
			out, _ := json.Marshal(parsed)
			for v := range seen {
				*matched = append(*matched, v)
			}
			return string(out)
		}
		return input
	}

	result := input
	for _, v := range vals {
		if strings.Contains(result, v) {
			result = strings.ReplaceAll(result, v, "****")
			seen[v] = struct{}{}
		}
	}
	for v := range seen {
		*matched = append(*matched, v)
	}
	return result
}

// redactJSONValue walks a parsed JSON tree and replaces secrets
// within string values only. Returns true if any replacement occurred.
func (s *MLScanner) redactJSONValue(v *interface{}, vals []string, seen map[string]struct{}) bool {
	switch ptr := (*v).(type) {
	case string:
		changed := false
		for _, secret := range vals {
			if strings.Contains(ptr, secret) {
				ptr = strings.ReplaceAll(ptr, secret, "****")
				seen[secret] = struct{}{}
				changed = true
			}
		}
		if changed {
			*v = ptr
		}
		return changed
	case map[string]interface{}:
		changed := false
		for k, val := range ptr {
			if s.redactJSONValue(&val, vals, seen) {
				ptr[k] = val
				changed = true
			}
		}
		return changed
	case []interface{}:
		changed := false
		for i, val := range ptr {
			if s.redactJSONValue(&val, vals, seen) {
				ptr[i] = val
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

// AddDynamic adds secret values to the dynamic store.
// Secrets already present in the static or dynamic store are silently skipped.
// This is called by the file watcher when credential files change at runtime.
func (s *MLScanner) AddDynamic(secrets ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, secret := range secrets {
		if s.addDynamicUnsafe(secret) && s.log != nil {
			fmt.Fprintf(s.log, "[aibodyguard] added dynamic secret: %s\n", secret)
		}
	}
}

// addDynamicUnsafe adds one secret to the dynamic store.
// Caller must hold s.mu write lock.
// Returns true if the secret was actually added (not a duplicate).
func (s *MLScanner) addDynamicUnsafe(secret string) bool {
	if _, ok := s.static[secret]; ok {
		return false
	}
	if _, ok := s.dynamic[secret]; ok {
		return false
	}
	s.dynamic[secret] = struct{}{}
	return true
}
