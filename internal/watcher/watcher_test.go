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

func TestScanReturnsNoNewSecretsOnFirstCall(t *testing.T) {
	// First call (in New) scans and caches. Second call should find nothing new.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("PASSWORD=test1234\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	secrets, err := w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 new secrets on unchanged file, got %d: %v", len(secrets), secrets)
	}
}

func TestScanDetectsNewSecretInChangedFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("PASSWORD=old1234\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	// Modify the file with a new secret (must pass IsLikelySecret)
	os.WriteFile(envPath, []byte("PASSWORD=old1234\nNEW_KEY=sk-new-key-abc123\n"), 0644)

	secrets, err := w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
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

	// Third call — no more new secrets
	secrets, err = w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 new secrets on third call, got %d: %v", len(secrets), secrets)
	}
}

func TestScanDetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("PASSWORD=first123\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	// Create a new credential file (value must pass IsLikelySecret)
	os.WriteFile(filepath.Join(dir, ".env.production"), []byte("API_KEY=prod-key-789xyz\n"), 0644)

	secrets, err := w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
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

func TestScanSkipsSourceCodeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("var x = \"not-a-secret-123\"\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=real-value\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	secrets, err := w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, s := range secrets {
		if strings.Contains(s, "not-a-secret-123") {
			t.Errorf("source code value should not be returned: %s", s)
		}
	}
}

func TestScanSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, ".env"), []byte("FAKE=skip-this-123\n"), 0644)

	// Real project .env
	os.WriteFile(filepath.Join(dir, ".env"), []byte("REAL=actual-value\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	secrets, err := w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, s := range secrets {
		if s == "skip-this-123" {
			t.Errorf("skip-this-123 from node_modules should not be returned")
		}
	}
}

func TestScanDeletedFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("KEY=value123\n"), 0644)

	w, err := watcher.New(dir, nil)
	if err != nil {
		t.Fatalf("watcher.New: %v", err)
	}

	// Delete the file
	os.Remove(envPath)

	secrets, err := w.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 new secrets after file deletion, got %d: %v", len(secrets), secrets)
	}
}
