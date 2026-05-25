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
# commented_key: should-not-appear
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
	// commented lines must NOT appear in ParseYAMLFile result
	if _, ok := got["commented_key"]; ok {
		t.Error("commented key should not appear in ParseYAMLFile")
	}
}

func TestParseCommentedYAMLFile(t *testing.T) {
	content := `
database:
  password: active-secret-password
  # host: localhost
#  api_key: sk-commented-key-abc123
# spring.kafka.properties.ssl.keystore.password: kafka-keystore-secret99
# just a plain prose comment
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, "secrets.yaml")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ParseCommentedYAMLFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"host":    "localhost",
		"api_key": "sk-commented-key-abc123",
		"spring.kafka.properties.ssl.keystore.password": "kafka-keystore-secret99",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %q: got %q, want %q", k, got[k], want)
		}
	}

	// active line must NOT appear in ParseCommentedYAMLFile result
	if _, ok := got["database.password"]; ok {
		t.Error("active line should not appear in ParseCommentedYAMLFile")
	}
	// plain prose should not produce a key
	for k := range got {
		if k == "just a plain prose comment" {
			t.Errorf("plain comment should not produce key %q", k)
		}
	}
}
