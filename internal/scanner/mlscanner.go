package scanner

import (
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
func (s *MLScanner) Redact(input string) (string, []string) {
	s.discover(input)

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
	result := input
	for _, v := range vals {
		if strings.Contains(result, v) {
			result = strings.ReplaceAll(result, v, "****")
			matched = append(matched, v)
		}
	}

	sort.Strings(matched)
	return result, matched
}

// discover runs the ML detector on input and adds any new secrets
// not already in the static or dynamic store.
func (s *MLScanner) discover(input string) {
	if s.det == nil || !s.det.Available() {
		return
	}
	newSecrets, err := s.det.DetectFromContent(input)
	if err != nil {
		if s.log != nil {
			fmt.Fprintf(s.log, "[aibodyguard] ML detection error: %v\n", err)
		}
		return
	}
	s.mu.Lock()
	for _, secret := range newSecrets {
		if _, ok := s.static[secret]; ok {
			continue
		}
		if _, ok := s.dynamic[secret]; ok {
			continue
		}
		s.dynamic[secret] = struct{}{}
		if s.log != nil {
			fmt.Fprintf(s.log, "[aibodyguard] ML discovered new secret at runtime: %s\n", secret)
		}
	}
	s.mu.Unlock()
}
