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
	// commented lines must NOT appear in ParseEnvFile result
	if _, ok := got["DB_PASSWORD_COMMENTED"]; ok {
		t.Error("commented key should not appear in ParseEnvFile")
	}
}

func TestParseCommentedEnvFile(t *testing.T) {
	content := `
APP_NAME=active-value
#DB_PASSWORD=supersecret123
# KAFKA_SSL_KEY=kafka-secret-key-abc
#QUOTED="commented quoted value"
## double-hash still valid
# just a plain comment
#spring.kafka.properties.security.protocol=SSL
! not a comment marker for =
!BANG_KEY=bang-value
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, ".env")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := ParseCommentedEnvFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"DB_PASSWORD":  "supersecret123",
		"KAFKA_SSL_KEY": "kafka-secret-key-abc",
		"QUOTED":       "commented quoted value",
		"spring.kafka.properties.security.protocol": "SSL",
		"BANG_KEY": "bang-value",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %q: got %q, want %q", k, got[k], want)
		}
	}

	// active line must NOT appear in ParseCommentedEnvFile result
	if _, ok := got["APP_NAME"]; ok {
		t.Error("active line should not appear in ParseCommentedEnvFile")
	}
	// plain prose comment should not produce a key
	for k := range got {
		if k == "just a plain comment" {
			t.Errorf("plain comment should not produce key %q", k)
		}
	}
}
