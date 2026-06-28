package watcher_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/watcher"
)

func TestNewWatcher(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("PASSWORD=test1234\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil watcher")
	}
}

func TestCheckReturnsNoNewSecretsOnSecondCall(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("PASSWORD=test1234\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	secrets, err := w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 new secrets on unchanged file, got %d: %v", len(secrets), secrets)
	}
}

func TestCheckDetectsNewSecretInChangedFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("PASSWORD=old1234\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	os.WriteFile(envPath, []byte("PASSWORD=old1234\nNEW_KEY=sk-new-key-abc123\n"), 0644)

	secrets, err := w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	found := false
	for _, s := range secrets {
		if s == "sk-new-key-abc123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sk-new-key-abc123 in new secrets, got %v", secrets)
	}

	secrets, err = w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 new secrets on third call, got %d: %v", len(secrets), secrets)
	}
}

func TestCheckDetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("PASSWORD=first123\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	os.WriteFile(filepath.Join(dir, ".env.production"), []byte("API_KEY=prod-key-789xyz\n"), 0644)

	secrets, err := w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	found := false
	for _, s := range secrets {
		if s == "prod-key-789xyz" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected prod-key-789xyz in new secrets, got %v", secrets)
	}
}

func TestCheckSkipsSourceCodeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("var x = \"not-a-secret-123\"\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=real-value\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	secrets, err := w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	for _, s := range secrets {
		if strings.Contains(s, "not-a-secret-123") {
			t.Errorf("source code value should not be returned: %s", s)
		}
	}
}

func TestCheckSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, ".env"), []byte("FAKE=skip-this-123\n"), 0644)

	os.WriteFile(filepath.Join(dir, ".env"), []byte("REAL=actual-value\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	secrets, err := w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	for _, s := range secrets {
		if s == "skip-this-123" {
			t.Errorf("skip-this-123 from node_modules should not be returned")
		}
	}
}

func TestCheckDeletedFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("KEY=value123\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	os.Remove(envPath)

	secrets, err := w.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 new secrets after file deletion, got %d: %v", len(secrets), secrets)
	}
}
