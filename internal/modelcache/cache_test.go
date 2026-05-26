package modelcache_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

func TestDefaultCacheDir(t *testing.T) {
	dir := modelcache.DefaultCacheDir()
	if dir == "" {
		t.Error("DefaultCacheDir() returned empty string")
	}
}

func TestModelPath(t *testing.T) {
	p := modelcache.ModelPath("/tmp/testcache")
	expected := filepath.Join("/tmp/testcache", "models", "model.onnx")
	if p != expected {
		t.Errorf("ModelPath: got %q, want %q", p, expected)
	}
}

func TestVocabPath(t *testing.T) {
	p := modelcache.VocabPath("/tmp/testcache")
	expected := filepath.Join("/tmp/testcache", "models", "vocab.txt")
	if p != expected {
		t.Errorf("VocabPath: got %q, want %q", p, expected)
	}
}

func TestIsReadyReturnsFalseWhenMissing(t *testing.T) {
	dir, _ := os.MkdirTemp("", "aibgtest*")
	defer os.RemoveAll(dir)
	if modelcache.IsReady(dir) {
		t.Error("IsReady should return false for empty cache dir")
	}
}

func TestIsReadyReturnsTrueWhenFilesExist(t *testing.T) {
	dir, _ := os.MkdirTemp("", "aibgtest*")
	defer os.RemoveAll(dir)

	modelsDir := filepath.Join(dir, "models")
	os.MkdirAll(modelsDir, 0755)
	os.WriteFile(filepath.Join(modelsDir, "model.onnx"), []byte("stub"), 0644)
	os.WriteFile(filepath.Join(modelsDir, "vocab.txt"), []byte("stub"), 0644)

	libDir := filepath.Join(dir, "lib")
	os.MkdirAll(libDir, 0755)
	libName := modelcache.LibName()
	os.WriteFile(filepath.Join(libDir, libName), []byte("stub"), 0644)

	if !modelcache.IsReady(dir) {
		t.Error("IsReady should return true when all files exist")
	}
}
