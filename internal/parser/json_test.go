package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONFile(t *testing.T) {
	content := `{
  "database": {
    "password": "db-secret-password",
    "host": "localhost"
  },
  "api_key": "sk-abc123xyz456",
  "port": 5432,
  "enabled": true
}`
	tmp := t.TempDir()
	f := filepath.Join(tmp, "credentials.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ParseJSONFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"database.password": "db-secret-password",
		"database.host":     "localhost",
		"api_key":           "sk-abc123xyz456",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %s: got %q, want %q", k, got[k], want)
		}
	}
	// non-string values should not be present
	if _, ok := got["port"]; ok {
		t.Error("numeric value should be excluded")
	}
}
