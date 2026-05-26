package mlmodel_test

import (
	"os"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/mlmodel"
	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

func TestNewModelUnavailableWhenCacheMissing(t *testing.T) {
	dir, _ := os.MkdirTemp("", "aibgtest*")
	defer os.RemoveAll(dir)

	m, err := mlmodel.New(dir)
	if err == nil {
		t.Error("expected error when cache is missing")
		m.Close()
	}
}

func TestAvailableReturnsFalseBeforeInit(t *testing.T) {
	var m *mlmodel.Model // nil model
	if m.Available() {
		t.Error("nil model should not be available")
	}
}

// TestPredictWithRealModel is skipped unless the model is cached.
func TestPredictWithRealModel(t *testing.T) {
	cacheDir := modelcache.DefaultCacheDir()
	if !modelcache.IsReady(cacheDir) {
		t.Skip("model not cached, skipping integration test")
	}

	m, err := mlmodel.New(cacheDir)
	if err != nil {
		t.Fatalf("mlmodel.New: %v", err)
	}
	defer m.Close()

	if !m.Available() {
		t.Fatal("model should be available after successful New()")
	}

	// Minimal input: [CLS]=101, hello=7592, world=2088, [SEP]=102
	tokenIDs := []int64{101, 7592, 2088, 102}
	mask := []int64{1, 1, 1, 1}

	scores, err := m.Predict(tokenIDs, mask)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(scores) != len(tokenIDs) {
		t.Errorf("expected %d scores, got %d", len(tokenIDs), len(scores))
	}
}
