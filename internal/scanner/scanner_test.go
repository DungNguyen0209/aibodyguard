package scanner

import (
	"strings"
	"testing"
)

func TestScanAndRedact(t *testing.T) {
	secrets := map[string]string{
		"DB_PASSWORD":  "supersecret123",
		"API_KEY":      "sk-abc123xyz456",
		"database.url": "postgres://supersecret123@localhost/db",
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"my password is supersecret123 and key sk-abc123xyz456"}]}`
	result, redacted := s.Redact(body)

	if strings.Contains(result, "supersecret123") {
		t.Error("supersecret123 should be redacted")
	}
	if strings.Contains(result, "sk-abc123xyz456") {
		t.Error("sk-abc123xyz456 should be redacted")
	}
	// Each matched secret should be replaced with ****
	count := strings.Count(result, "****")
	if count != 2 {
		t.Errorf("expected 2 occurrences of ****, got %d in: %s", count, result)
	}

	redactedSet := make(map[string]bool)
	for _, k := range redacted {
		redactedSet[k] = true
	}
	if !redactedSet["DB_PASSWORD"] {
		t.Error("DB_PASSWORD should be in redacted list")
	}
	if !redactedSet["API_KEY"] {
		t.Error("API_KEY should be in redacted list")
	}
	if len(redacted) != 2 {
		t.Errorf("expected 2 redacted keys, got %d: %v", len(redacted), redacted)
	}
}

func TestScanAndRedactNoMatch(t *testing.T) {
	secrets := map[string]string{
		"DB_PASSWORD": "supersecret123",
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"nothing secret here"}]}`
	result, redacted := s.Redact(body)

	if result != body {
		t.Error("body should be unchanged when no secrets found")
	}
	if len(redacted) != 0 {
		t.Errorf("expected 0 redacted keys, got %d", len(redacted))
	}
}
