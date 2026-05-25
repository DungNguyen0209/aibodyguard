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

func TestParseYAMLFile_CommentedOutCredentials(t *testing.T) {
	content := `
database:
  password: active-secret-password
  # host: localhost
#  api_key: sk-commented-key-abc123
# spring.kafka.properties.ssl.keystore.password: kafka-keystore-secret99
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

	// active line still works
	if got["database.password"] != "active-secret-password" {
		t.Errorf("database.password: got %q, want %q", got["database.password"], "active-secret-password")
	}
	// commented-out key: value lines should also be parsed
	if got["api_key"] != "sk-commented-key-abc123" {
		t.Errorf("api_key: got %q, want %q", got["api_key"], "sk-commented-key-abc123")
	}
	if got["spring.kafka.properties.ssl.keystore.password"] != "kafka-keystore-secret99" {
		t.Errorf("spring.kafka...: got %q, want %q",
			got["spring.kafka.properties.ssl.keystore.password"], "kafka-keystore-secret99")
	}
}
