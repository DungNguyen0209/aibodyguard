package uninstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/uninstall"
)

func TestRemoveCacheDir(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "aibodyguard")
	if err := os.MkdirAll(filepath.Join(cacheDir, "models"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "models", "model.onnx"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	removed, err := uninstall.RemoveCacheDir(cacheDir)
	if err != nil {
		t.Fatalf("RemoveCacheDir: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("expected cache dir to be gone")
	}
}

func TestRemoveCacheDirMissing(t *testing.T) {
	removed, err := uninstall.RemoveCacheDir("/nonexistent/path/aibodyguard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Error("expected removed=false when dir does not exist")
	}
}

func TestRemoveTempFiles(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "aibodyguard.log"),
		filepath.Join(dir, "aibodyguard-ca.pem"),
	}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	removed := uninstall.RemoveTempFiles(paths)
	if len(removed) != 2 {
		t.Errorf("expected 2 removed, got %d: %v", len(removed), removed)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted", p)
		}
	}
}
