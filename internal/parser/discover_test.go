package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSecrets(t *testing.T) {
	tmp := t.TempDir()

	// .env file
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("SECRET_KEY=abc12345678\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// JSON file
	if err := os.WriteFile(filepath.Join(tmp, "creds.json"), []byte(`{"token":"json-token-xyz9876"}`), 0644); err != nil {
		t.Fatalf("failed to write creds.json: %v", err)
	}

	// YAML file
	if err := os.WriteFile(filepath.Join(tmp, "secrets.yaml"), []byte("api_key: yaml-key-qwerty123\n"), 0644); err != nil {
		t.Fatalf("failed to write secrets.yaml: %v", err)
	}

	// properties file
	if err := os.WriteFile(filepath.Join(tmp, "app.properties"), []byte("db.password=props-pass-abc456\n"), 0644); err != nil {
		t.Fatalf("failed to write app.properties: %v", err)
	}

	// node_modules should be skipped
	if err := os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755); err != nil {
		t.Fatalf("failed to create node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "node_modules", "secret.env"), []byte("SKIP=should-not-load-this\n"), 0644); err != nil {
		t.Fatalf("failed to write node_modules/secret.env: %v", err)
	}

	secrets, err := DiscoverSecrets(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if secrets["SECRET_KEY"] != "abc12345678" {
		t.Errorf("missing SECRET_KEY, got: %v", secrets["SECRET_KEY"])
	}
	if secrets["token"] != "json-token-xyz9876" {
		t.Errorf("missing token, got: %v", secrets["token"])
	}
	if secrets["api_key"] != "yaml-key-qwerty123" {
		t.Errorf("missing api_key, got: %v", secrets["api_key"])
	}
	if secrets["db.password"] != "props-pass-abc456" {
		t.Errorf("missing db.password, got: %v", secrets["db.password"])
	}
	if _, ok := secrets["SKIP"]; ok {
		t.Error("node_modules secret should not be loaded")
	}
}
