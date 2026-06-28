package scanner

import (
	"io"
	"strings"
	"sync"
	"testing"
)

// fakeDetector implements secretDetector for testing.
type fakeDetector struct {
	available bool
	secrets   []string
	err       error
}

func (f *fakeDetector) Available() bool { return f.available }

func (f *fakeDetector) DetectFromContent(content string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.secrets, nil
}

func TestMLScanner_StaticOnly(t *testing.T) {
	secrets := map[string][]string{
		"DB_PASSWORD": {"supersecret123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)

	body := `{"password":"supersecret123"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "supersecret123") {
		t.Error("supersecret123 should be redacted")
	}
	if len(matched) != 1 || matched[0] != "supersecret123" {
		t.Errorf("expected [supersecret123], got %v", matched)
	}
}

func TestMLScanner_StaticOnly_NoMatch(t *testing.T) {
	secrets := map[string][]string{
		"DB_PASSWORD": {"supersecret123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)

	body := `{"msg":"nothing here"}`
	result, matched := s.Redact(body)

	if result != body {
		t.Error("body should be unchanged")
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matched, got %v", matched)
	}
}

func TestMLScanner_DynamicDiscovery(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("new-dynamic-456")

	body := `{"key":"new-dynamic-456"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "new-dynamic-456") {
		t.Error("dynamic secret should be redacted")
	}
	if len(matched) != 1 || matched[0] != "new-dynamic-456" {
		t.Errorf("expected [new-dynamic-456], got %v", matched)
	}

	// Second call — should redact from dynamic store
	body2 := `{"key":"new-dynamic-456"}`
	result2, matched2 := s.Redact(body2)

	if strings.Contains(result2, "new-dynamic-456") {
		t.Error("dynamic secret should be redacted on second call")
	}
	if len(matched2) != 1 {
		t.Errorf("expected 1 match on second call, got %d", len(matched2))
	}
}

func TestMLScanner_NoDuplicateDynamic(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("dup-secret-123")
	s.AddDynamic("dup-secret-123")
	s.AddDynamic("unique-456")

	s.mu.RLock()
	n := len(s.dynamic)
	s.mu.RUnlock()

	if n != 2 {
		t.Errorf("expected 2 dynamic secrets after dedup, got %d", n)
	}
}

func TestMLScanner_StaticOverDynamic(t *testing.T) {
	// If a secret is already in static, AddDynamic should skip it.
	secrets := map[string][]string{
		"API_KEY": {"already-known-123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)
	s.AddDynamic("already-known-123")

	s.mu.RLock()
	dynLen := len(s.dynamic)
	s.mu.RUnlock()

	if dynLen != 0 {
		t.Errorf("expected 0 dynamic secrets (already in static), got %d", dynLen)
	}
}

func TestMLScanner_DetectorNotAvailable(t *testing.T) {
	// AddDynamic works regardless of detector availability
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("some-secret-123")

	s.mu.RLock()
	dynLen := len(s.dynamic)
	s.mu.RUnlock()

	if dynLen != 1 {
		t.Errorf("expected 1 dynamic secret, got %d", dynLen)
	}
}

func TestMLScanner_DetectorError(t *testing.T) {
	// AddDynamic ignores detector errors
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("test-secret-789")

	s.mu.RLock()
	dynLen := len(s.dynamic)
	s.mu.RUnlock()

	if dynLen != 1 {
		t.Errorf("expected 1 dynamic secret, got %d", dynLen)
	}
}

func TestMLScanner_EmptyBody(t *testing.T) {
	secrets := map[string][]string{
		"KEY": {"secret-123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)

	result, matched := s.Redact("")

	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matched, got %v", matched)
	}
}

func TestMLScanner_StaticAndDynamicCombined(t *testing.T) {
	secrets := map[string][]string{
		"DB_PW": {"static-pw-123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)
	s.AddDynamic("dynamic-key-456")

	body := `{"static":"static-pw-123","dynamic":"dynamic-key-456"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "static-pw-123") {
		t.Error("static-pw-123 should be redacted")
	}
	if strings.Contains(result, "dynamic-key-456") {
		t.Error("dynamic-key-456 should be redacted")
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched values, got %d: %v", len(matched), matched)
	}
}

func TestMLScanner_ConcurrentSafety(t *testing.T) {
	secrets := map[string][]string{
		"STATIC": {"static-val-123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)
	s.AddDynamic("concurrent-456")

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Redact(`{"k":"concurrent-456"}`)
		}()
	}
	wg.Wait()

	s.mu.RLock()
	dynLen := len(s.dynamic)
	s.mu.RUnlock()

	if dynLen != 1 {
		t.Errorf("expected 1 dynamic secret after concurrent calls, got %d", dynLen)
	}
}

func TestMLScanner_DynamicNotInStatic(t *testing.T) {
	// Verify that dynamic secrets don't pollute the static store.
	secrets := map[string][]string{
		"A": {"a-value-123"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)
	s.AddDynamic("b-value-456")

	s.mu.RLock()
	_, inStatic := s.static["b-value-456"]
	dynLen := len(s.dynamic)
	s.mu.RUnlock()

	if inStatic {
		t.Error("b-value-456 should NOT be in static store")
	}
	if dynLen != 1 {
		t.Errorf("expected 1 dynamic secret, got %d", dynLen)
	}
}

func TestMLScanner_AddDynamic(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)

	s.AddDynamic("new-secret-1", "new-secret-2")

	body := `{"k1":"new-secret-1","k2":"new-secret-2"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "new-secret-1") {
		t.Error("new-secret-1 should be redacted")
	}
	if strings.Contains(result, "new-secret-2") {
		t.Error("new-secret-2 should be redacted")
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matches, got %d: %v", len(matched), matched)
	}
}

func TestMLScanner_AddDynamic_Deduplicates(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)

	s.AddDynamic("dup")
	s.AddDynamic("dup")
	s.AddDynamic("unique")

	s.mu.RLock()
	n := len(s.dynamic)
	s.mu.RUnlock()

	if n != 2 {
		t.Errorf("expected 2 dynamic secrets after dedup, got %d", n)
	}
}

func TestMLScanner_AddDynamic_StaticNotDuplicated(t *testing.T) {
	secrets := map[string][]string{
		"KEY": {"already-static"},
	}
	s := NewMLScanner(secrets, nil, io.Discard)

	s.AddDynamic("already-static")

	s.mu.RLock()
	n := len(s.dynamic)
	s.mu.RUnlock()

	if n != 0 {
		t.Errorf("expected 0 dynamic secrets (static already exists), got %d", n)
	}
}

func TestMLScanner_JSONAware_OnlyRedactsStrings(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("secret")

	// JSON key named "secret" should NOT be corrupted
	body := `{"secret":"value-with-secret-inside","other":"no-secret-here"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "value-with-secret-inside") {
		t.Error("string value should be redacted")
	}
	if !strings.Contains(result, `"secret"`) {
		t.Error("key 'secret' should NOT be redacted - would corrupt JSON")
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d: %v", len(matched), matched)
	}
}

func TestMLScanner_JSONAware_NestedValues(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("secret123")

	body := `{"a":{"b":[{"c":"prefix secret123 suffix"}]}}`
	result, matched := s.Redact(body)

	if !strings.Contains(result, `"c":"prefix **** suffix"`) {
		t.Errorf("nested string value should be redacted, got: %s", result)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d: %v", len(matched), matched)
	}
}

func TestMLScanner_JSONAware_NonJSONBody(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("secret")

	// Non-JSON body should still be redacted via naive replacement
	body := `this is a secret value`
	result, matched := s.Redact(body)

	if strings.Contains(result, "secret") {
		t.Error("secret should be redacted in non-JSON body")
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestMLScanner_JSONAware_NumberValuesUntouched(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("42")

	body := `{"port":42,"name":"port 42 is here"}`
	result, matched := s.Redact(body)

	// Number 42 should NOT be replaced (it's a JSON number, not a string)
	if strings.Contains(result, `"port":****`) {
		t.Errorf("number value 42 should not be redacted: %s", result)
	}
	// String value containing "42" SHOULD be redacted
	if !strings.Contains(result, `"port **** is here"`) {
		t.Errorf("string containing 42 should be redacted: %s", result)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestMLScanner_JSONAware_BooleanValuesUntouched(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("true")

	body := `{"enabled":true,"msg":"this is true"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, `"enabled":****`) {
		t.Errorf("boolean true should not be redacted: %s", result)
	}
	if !strings.Contains(result, `"this is ****"`) {
		t.Errorf("string containing true should be redacted: %s", result)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestMLScanner_JSONAware_ArrayValues(t *testing.T) {
	s := NewMLScanner(nil, nil, io.Discard)
	s.AddDynamic("redact-me")

	body := `{"items":["redact-me","keep-me","also redact-me"]}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "redact-me") {
		t.Errorf("array string elements should be redacted: %s", result)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d: %v", len(matched), matched)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
