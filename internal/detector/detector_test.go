package detector_test

import (
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/detector"
	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

func TestDetectorUnavailableWithNoModel(t *testing.T) {
	d, err := detector.New("/nonexistent/cache/dir")
	if err == nil {
		t.Error("expected error for missing cache dir")
	}
	if d != nil && d.Available() {
		t.Error("detector should not be available with missing model")
	}
}

func TestDetectSecretInPassword(t *testing.T) {
	cacheDir := modelcache.DefaultCacheDir()
	if !modelcache.IsReady(cacheDir) {
		t.Skip("model not cached, skipping integration test")
	}

	d, err := detector.New(cacheDir)
	if err != nil {
		t.Fatalf("detector.New: %v", err)
	}
	defer d.Close()

	content := "POSTGRES_PASSWORD=wb2000\nPOSTGRES_PORT=5432\n"
	secrets, err := d.DetectFromContent(content)
	if err != nil {
		t.Fatalf("DetectFromContent: %v", err)
	}

	found := false
	for _, s := range secrets {
		if s == "wb2000" {
			found = true
		}
		if s == "5432" {
			t.Errorf("5432 (port) should not be detected as a secret")
		}
	}
	if !found {
		t.Errorf("expected wb2000 to be detected as a secret, got: %v", secrets)
	}
}

func TestDetectCommentedSecret(t *testing.T) {
	cacheDir := modelcache.DefaultCacheDir()
	if !modelcache.IsReady(cacheDir) {
		t.Skip("model not cached, skipping integration test")
	}

	d, err := detector.New(cacheDir)
	if err != nil {
		t.Fatalf("detector.New: %v", err)
	}
	defer d.Close()

	content := "# OLD_API_KEY=sk-proj-abc123xyzABC\n"
	secrets, err := d.DetectFromContent(content)
	if err != nil {
		t.Fatalf("DetectFromContent: %v", err)
	}

	found := false
	for _, s := range secrets {
		if s == "sk-proj-abc123xyzABC" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sk-proj-abc123xyzABC to be detected, got: %v", secrets)
	}
}
