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

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checkContains := func(key, want string) {
		t.Helper()
		vals, ok := secrets[key]
		if !ok {
			t.Errorf("key %q not found in secrets", key)
			return
		}
		for _, v := range vals {
			if v == want {
				return
			}
		}
		t.Errorf("key %q does not contain value %q, got: %v", key, want, vals)
	}

	checkContains("SECRET_KEY", "abc12345678")
	checkContains("token", "json-token-xyz9876")
	checkContains("api_key", "yaml-key-qwerty123")
	checkContains("db.password", "props-pass-abc456")

	if _, ok := secrets["SKIP"]; ok {
		t.Error("node_modules secret should not be loaded")
	}
}

func TestDiscoverSecrets_DuplicateKey(t *testing.T) {
	tmp := t.TempDir()

	sub1 := filepath.Join(tmp, "svc1")
	sub2 := filepath.Join(tmp, "svc2")
	if err := os.MkdirAll(sub1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub2, 0755); err != nil {
		t.Fatal(err)
	}

	// Same key, different values in two different files
	if err := os.WriteFile(filepath.Join(sub1, "secrets.yaml"), []byte("JDBC_URL: jdbc:mysql://host1/db1abc12345\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "secrets.yaml"), []byte("JDBC_URL: jdbc:mysql://host2/db2abc12345\n"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vals := secrets["JDBC_URL"]
	if len(vals) != 2 {
		t.Errorf("expected 2 values for JDBC_URL, got %d: %v", len(vals), vals)
	}

	found1, found2 := false, false
	for _, v := range vals {
		if v == "jdbc:mysql://host1/db1abc12345" {
			found1 = true
		}
		if v == "jdbc:mysql://host2/db2abc12345" {
			found2 = true
		}
	}
	if !found1 {
		t.Error("missing jdbc:mysql://host1/db1abc12345")
	}
	if !found2 {
		t.Error("missing jdbc:mysql://host2/db2abc12345")
	}
}

func TestDiscoverSecrets_DuplicateValue(t *testing.T) {
	tmp := t.TempDir()

	sub1 := filepath.Join(tmp, "svc1")
	sub2 := filepath.Join(tmp, "svc2")
	os.MkdirAll(sub1, 0755)
	os.MkdirAll(sub2, 0755)

	// Same key, same value in two files — should deduplicate
	if err := os.WriteFile(filepath.Join(sub1, "secrets.yaml"), []byte("API_KEY: sk-abc123xyz456\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "secrets.yaml"), []byte("API_KEY: sk-abc123xyz456\n"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatal(err)
	}

	vals := secrets["API_KEY"]
	if len(vals) != 1 {
		t.Errorf("expected 1 deduplicated value for API_KEY, got %d: %v", len(vals), vals)
	}
}

func TestDiscoverSecrets_SettingFilename(t *testing.T) {
	tmp := t.TempDir()

	// YAML with "setting" in name
	if err := os.WriteFile(filepath.Join(tmp, "appsettings-prod.yaml"), []byte("db_password: prodPass9876xyz\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// JSON with "setting" in name
	if err := os.WriteFile(filepath.Join(tmp, "site-settings.json"), []byte(`{"api_token":"tok-settingTest1234"}`), 0644); err != nil {
		t.Fatal(err)
	}

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checkContains := func(key, want string) {
		t.Helper()
		vals, ok := secrets[key]
		if !ok {
			t.Errorf("key %q not found in secrets", key)
			return
		}
		for _, v := range vals {
			if v == want {
				return
			}
		}
		t.Errorf("key %q does not contain %q, got: %v", key, want, vals)
	}

	checkContains("db_password", "prodPass9876xyz")
	checkContains("api_token", "tok-settingTest1234")
}
