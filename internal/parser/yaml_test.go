package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseYAMLFile(t *testing.T) {
	content := `
database:
  password: yaml-secret-password
  host: localhost
api_key: sk-yaml-key-abc123
port: 5432
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, "secrets.yaml")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ParseYAMLFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"database.password": "yaml-secret-password",
		"database.host":     "localhost",
		"api_key":           "sk-yaml-key-abc123",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %s: got %q, want %q", k, got[k], want)
		}
	}
	if _, ok := got["port"]; ok {
		t.Error("numeric value should be excluded")
	}
}
