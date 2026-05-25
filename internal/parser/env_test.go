package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	content := `
# comment line
APP_NAME=myapp
DB_PASSWORD=supersecret123
QUOTED_VAL="quoted value here"
SINGLE_QUOTED='single quoted'
EMPTY_VAL=
! another comment
JAVA_PROP=somevalue
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, ".env")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ParseEnvFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"APP_NAME":      "myapp",
		"DB_PASSWORD":   "supersecret123",
		"QUOTED_VAL":    "quoted value here",
		"SINGLE_QUOTED": "single quoted",
		"JAVA_PROP":     "somevalue",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %s: got %q, want %q", k, got[k], want)
		}
	}
	if _, ok := got["EMPTY_VAL"]; ok {
		t.Error("empty value should be excluded")
	}
}

func TestParseEnvFile_CommentedOutCredentials(t *testing.T) {
	content := `
# normal comment — no key=value, should be skipped
APP_NAME=myapp
#DB_PASSWORD=supersecret123
# KAFKA_SSL_KEY=kafka-secret-key-abc
#QUOTED="commented quoted value"
## double-hash comment without key=value should be skipped
# just a plain comment
#spring.kafka.properties.security.protocol=SSL
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, ".env")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ParseEnvFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// commented-out key=value lines should still be parsed
	if got["DB_PASSWORD"] != "supersecret123" {
		t.Errorf("DB_PASSWORD: got %q, want %q", got["DB_PASSWORD"], "supersecret123")
	}
	if got["KAFKA_SSL_KEY"] != "kafka-secret-key-abc" {
		t.Errorf("KAFKA_SSL_KEY: got %q, want %q", got["KAFKA_SSL_KEY"], "kafka-secret-key-abc")
	}
	if got["QUOTED"] != "commented quoted value" {
		t.Errorf("QUOTED: got %q, want %q", got["QUOTED"], "commented quoted value")
	}
	if got["spring.kafka.properties.security.protocol"] != "SSL" {
		t.Errorf("spring.kafka.properties.security.protocol: got %q, want %q",
			got["spring.kafka.properties.security.protocol"], "SSL")
	}

	// plain comments with no key=value should not produce any key
	for k := range got {
		if k == "normal comment — no key=value" || k == "just a plain comment" {
			t.Errorf("plain comment should not produce key %q", k)
		}
	}
}

