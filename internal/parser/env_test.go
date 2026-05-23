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
