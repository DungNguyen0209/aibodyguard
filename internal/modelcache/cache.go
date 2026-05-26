package modelcache

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const (
	modelURL = "https://huggingface.co/AndrewAndrewsen/distilbert-secret-masker/resolve/main/model.onnx"
	vocabURL = "https://huggingface.co/AndrewAndrewsen/distilbert-secret-masker/resolve/main/vocab.txt"

	onnxRuntimeVersion = "1.25.0"
	onnxRuntimeBaseURL = "https://github.com/microsoft/onnxruntime/releases/download/v" + onnxRuntimeVersion + "/"
)

// DefaultCacheDir returns ~/.cache/aibodyguard.
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".aibodyguard-cache")
	}
	return filepath.Join(home, ".cache", "aibodyguard")
}

// ModelPath returns the path to model.onnx within cacheDir.
func ModelPath(cacheDir string) string {
	return filepath.Join(cacheDir, "models", "model.onnx")
}

// VocabPath returns the path to vocab.txt within cacheDir.
func VocabPath(cacheDir string) string {
	return filepath.Join(cacheDir, "models", "vocab.txt")
}

// LibPath returns the path to the onnxruntime shared library within cacheDir.
func LibPath(cacheDir string) string {
	return filepath.Join(cacheDir, "lib", LibName())
}

// LibName returns the platform-specific filename for the onnxruntime shared library.
func LibName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime." + onnxRuntimeVersion + ".dylib"
	case "windows":
		return "onnxruntime.dll"
	default:
		return "libonnxruntime.so." + onnxRuntimeVersion
	}
}

// libDownloadURL returns the download URL for the onnxruntime shared library archive.
func libDownloadURL() string {
	var archive string
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			archive = fmt.Sprintf("onnxruntime-osx-arm64-%s.tgz", onnxRuntimeVersion)
		} else {
			archive = fmt.Sprintf("onnxruntime-osx-x86_64-%s.tgz", onnxRuntimeVersion)
		}
	case "windows":
		archive = fmt.Sprintf("onnxruntime-win-x64-%s.zip", onnxRuntimeVersion)
	default:
		if runtime.GOARCH == "arm64" {
			archive = fmt.Sprintf("onnxruntime-linux-aarch64-%s.tgz", onnxRuntimeVersion)
		} else {
			archive = fmt.Sprintf("onnxruntime-linux-x64-%s.tgz", onnxRuntimeVersion)
		}
	}
	return onnxRuntimeBaseURL + archive
}

// IsReady returns true if all required cache files exist.
func IsReady(cacheDir string) bool {
	paths := []string{
		ModelPath(cacheDir),
		VocabPath(cacheDir),
		LibPath(cacheDir),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// EnsureReady downloads missing artifacts to cacheDir, printing progress to stdout.
// Returns nil if all artifacts are already present or successfully downloaded.
// Returns an error if any download fails — callers should fall back to heuristic detection.
func EnsureReady(cacheDir string) error {
	if err := os.MkdirAll(filepath.Join(cacheDir, "models"), 0755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, "lib"), 0755); err != nil {
		return fmt.Errorf("create lib dir: %w", err)
	}

	type artifact struct {
		name string
		url  string
		dest string
	}

	artifacts := []artifact{
		{"vocab.txt", vocabURL, VocabPath(cacheDir)},
		{"model.onnx (265MB)", modelURL, ModelPath(cacheDir)},
	}

	needsDownload := false
	for _, a := range artifacts {
		if _, err := os.Stat(a.dest); err != nil {
			needsDownload = true
			break
		}
	}
	if needsDownload {
		fmt.Println("  Downloading ML model (first run only)...")
	}

	for _, a := range artifacts {
		if _, err := os.Stat(a.dest); err == nil {
			continue // already cached
		}
		fmt.Printf("    %-40s ", a.name)
		if err := downloadFile(a.url, a.dest); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("download %s: %w", a.name, err)
		}
		fmt.Println("done")
	}

	// libonnxruntime is distributed as a tgz/zip archive requiring extraction.
	// We instruct users to place it manually until archive extraction is implemented.
	libDest := LibPath(cacheDir)
	if _, err := os.Stat(libDest); err != nil {
		fmt.Printf("  warning: onnxruntime shared library not found at %s\n", libDest)
		fmt.Printf("           Download from: %s\n", libDownloadURL())
		fmt.Printf("           Extract %s to: %s\n", LibName(), filepath.Dir(libDest))
		return fmt.Errorf("onnxruntime shared library missing: %s", libDest)
	}

	return nil
}

// downloadFile downloads url to destPath, writing to a temp file first to avoid
// partial writes if the download is interrupted.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp := destPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, destPath)
}
