package scanner

import (
	"strings"
	"testing"
)

func TestScanAndRedact(t *testing.T) {
	secrets := map[string][]string{
		"DB_PASSWORD":  {"supersecret123"},
		"API_KEY":      {"sk-abc123xyz456"},
		"database.url": {"postgres://supersecret123@localhost/db"},
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"my password is supersecret123 and key sk-abc123xyz456"}]}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "supersecret123") {
		t.Error("supersecret123 should be redacted")
	}
	if strings.Contains(result, "sk-abc123xyz456") {
		t.Error("sk-abc123xyz456 should be redacted")
	}

	count := strings.Count(result, "****")
	if count != 2 {
		t.Errorf("expected 2 occurrences of ****, got %d in: %s", count, result)
	}

	matchedSet := make(map[string]bool)
	for _, v := range matched {
		matchedSet[v] = true
	}
	if !matchedSet["supersecret123"] {
		t.Error("supersecret123 should be in matched list")
	}
	if !matchedSet["sk-abc123xyz456"] {
		t.Error("sk-abc123xyz456 should be in matched list")
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched values, got %d: %v", len(matched), matched)
	}
}

func TestScanAndRedactNoMatch(t *testing.T) {
	secrets := map[string][]string{
		"DB_PASSWORD": {"supersecret123"},
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"nothing secret here"}]}`
	result, matched := s.Redact(body)

	if result != body {
		t.Error("body should be unchanged when no secrets found")
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matched values, got %d", len(matched))
	}
}

func TestScanAndRedact_DuplicateKey(t *testing.T) {
	// Same key with two different values — both should be redacted
	secrets := map[string][]string{
		"JDBC_URL": {"jdbc:mysql://host1/db1abc12345", "jdbc:mysql://host2/db2abc12345"},
	}
	var s Scanner = New(secrets)

	body := `{"url1":"jdbc:mysql://host1/db1abc12345","url2":"jdbc:mysql://host2/db2abc12345"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "host1") {
		t.Error("host1 jdbc url should be redacted")
	}
	if strings.Contains(result, "host2") {
		t.Error("host2 jdbc url should be redacted")
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched values, got %d: %v", len(matched), matched)
	}
}

func TestScanAndRedact_DeduplicatesValues(t *testing.T) {
	// Same value under two different keys — should only redact once (not double-replace)
	secrets := map[string][]string{
		"KEY_A": {"sharedSecret123"},
		"KEY_B": {"sharedSecret123"},
	}
	var s Scanner = New(secrets)

	body := `{"val":"sharedSecret123"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "sharedSecret123") {
		t.Error("sharedSecret123 should be redacted")
	}
	count := strings.Count(result, "****")
	if count != 1 {
		t.Errorf("expected 1 occurrence of ****, got %d", count)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 matched value, got %d: %v", len(matched), matched)
	}
}
