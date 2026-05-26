# ML-Based Secret Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate the `distilbert-secret-masker` ONNX model into AIBodyguard for ML-based secret detection using a belt + suspenders + heuristic fallback approach.

**Architecture:** The distilbert-secret-masker model runs locally via `onnxruntime_go` (CGo bindings to Microsoft ONNX Runtime). On first run, the model (~265MB) and runtime library (~30MB) are downloaded and cached at `~/.cache/aibodyguard/`. Raw credential file content is tokenized with a Go WordPiece tokenizer, chunked into ≤510-token windows with 64-token overlap, and fed through the model. SECRET-labeled tokens are mapped back to character substrings. Commented-out lines are additionally stripped and re-fed. The heuristic `isLikelySecret` remains as a fallback.

**Tech Stack:** Go 1.22+, `github.com/yalue/onnxruntime_go`, ONNX Runtime v1.25.0 shared library, HuggingFace `distilbert-secret-masker` ONNX model.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `internal/tokenizer/wordpiece.go` | WordPiece tokenizer: load vocab.txt, encode text → token IDs + offsets |
| Create | `internal/tokenizer/wordpiece_test.go` | Tests for tokenizer |
| Create | `internal/chunker/chunker.go` | Split token sequences into ≤510-token overlapping windows |
| Create | `internal/chunker/chunker_test.go` | Tests for chunker |
| Create | `internal/mlmodel/model.go` | ONNX Runtime session: load, predict, close |
| Create | `internal/mlmodel/model_test.go` | Tests for model (skip if model not available) |
| Create | `internal/modelcache/cache.go` | Download + cache libonnxruntime, model.onnx, vocab.txt |
| Create | `internal/modelcache/cache_test.go` | Tests for cache path resolution |
| Create | `internal/detector/detector.go` | Pipeline: raw content → chunked inference → secret strings |
| Create | `internal/detector/detector_test.go` | Integration tests for detection |
| Modify | `internal/parser/discover.go` | Accept `*detector.Detector`, run belt+suspenders+fallback |
| Modify | `internal/parser/discover_test.go` | Update tests to pass nil detector |
| Modify | `cmd/aibodyguard/main.go` | Init cache, init detector, pass to DiscoverSecrets |

---

## Task 1: Feature Branch and Go Dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Verify you are on the feature branch**

```bash
git branch
```

Expected: `* feature/ml-secret-detection`

- [ ] **Step 2: Add onnxruntime_go dependency**

```bash
go get github.com/yalue/onnxruntime_go@latest
```

Expected: `go.mod` and `go.sum` updated with `github.com/yalue/onnxruntime_go`.

- [ ] **Step 3: Verify build still compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add onnxruntime_go dependency"
```

---

## Task 2: WordPiece Tokenizer

**Files:**
- Create: `internal/tokenizer/wordpiece.go`
- Create: `internal/tokenizer/wordpiece_test.go`

The WordPiece tokenizer converts a string into DistilBERT token IDs and records the character offset of each token so we can map SECRET labels back to the original text.

- [ ] **Step 1: Write the failing tests**

Create `internal/tokenizer/wordpiece_test.go`:

```go
package tokenizer_test

import (
	"os"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/tokenizer"
)

// TestEncodeKnownTokens uses a minimal in-memory vocab to verify encoding
// without requiring the real vocab.txt file.
func TestEncodeKnownTokens(t *testing.T) {
	// Write a minimal vocab file for testing
	vocabContent := "[PAD]\n[UNK]\n[CLS]\n[SEP]\n[MASK]\npassword\n=\nwb\n##2000\nhello\nworld\n"
	f, err := os.CreateTemp("", "vocab*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(vocabContent)
	f.Close()

	tok, err := tokenizer.NewWordPiece(f.Name())
	if err != nil {
		t.Fatalf("NewWordPiece: %v", err)
	}

	ids, mask, offsets, err := tok.Encode("hello world")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Should have at least 2 tokens (hello, world) plus CLS and SEP
	if len(ids) < 4 {
		t.Errorf("expected at least 4 token IDs, got %d", len(ids))
	}
	// First token must be [CLS] (id=2 in our vocab)
	if ids[0] != 2 {
		t.Errorf("expected ids[0]=2 ([CLS]), got %d", ids[0])
	}
	// Last token must be [SEP] (id=3)
	if ids[len(ids)-1] != 3 {
		t.Errorf("expected last id=3 ([SEP]), got %d", ids[len(ids)-1])
	}
	// Attention mask must be all 1s
	for i, m := range mask {
		if m != 1 {
			t.Errorf("mask[%d]=%d, want 1", i, m)
		}
	}
	// Offsets length must equal ids length
	if len(offsets) != len(ids) {
		t.Errorf("offsets len %d != ids len %d", len(offsets), len(ids))
	}
}

func TestEncodeUnknownFallsToUNK(t *testing.T) {
	vocabContent := "[PAD]\n[UNK]\n[CLS]\n[SEP]\n[MASK]\nhello\n"
	f, err := os.CreateTemp("", "vocab*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(vocabContent)
	f.Close()

	tok, _ := tokenizer.NewWordPiece(f.Name())
	ids, _, _, err := tok.Encode("xyzzy")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// xyzzy is unknown → should be [UNK] (id=1)
	// ids = [CLS, UNK, SEP]
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(ids))
	}
	if ids[1] != 1 {
		t.Errorf("expected ids[1]=1 ([UNK]), got %d", ids[1])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tokenizer/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement the tokenizer**

Create `internal/tokenizer/wordpiece.go`:

```go
package tokenizer

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// TokenOffset records the start and end byte positions in the original text
// for a single token. CLS and SEP tokens get offset {0, 0}.
type TokenOffset struct {
	Start int
	End   int
}

// WordPiece is a minimal DistilBERT-compatible tokenizer.
// It lowercases input, splits on whitespace and punctuation,
// then applies WordPiece sub-word splitting using vocab.txt.
type WordPiece struct {
	vocab   map[string]int64
	unkID   int64
	clsID   int64
	sepID   int64
}

// NewWordPiece loads a HuggingFace vocab.txt file and returns a WordPiece tokenizer.
func NewWordPiece(vocabPath string) (*WordPiece, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int64)
	var idx int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		token := sc.Text()
		vocab[token] = idx
		idx++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	unk, _ := vocab["[UNK]"]
	cls, _ := vocab["[CLS]"]
	sep, _ := vocab["[SEP]"]

	return &WordPiece{vocab: vocab, unkID: unk, clsID: cls, sepID: sep}, nil
}

// Encode tokenizes text and returns:
//   - tokenIDs: int64 slice with [CLS] prepended and [SEP] appended
//   - attentionMask: all 1s, same length as tokenIDs
//   - offsets: character byte offsets for each token (CLS/SEP get {0,0})
func (wp *WordPiece) Encode(text string) ([]int64, []int64, []TokenOffset, error) {
	lower := strings.ToLower(text)
	words, wordOffsets := tokenizeToWords(lower, text)

	var ids []int64
	var offsets []TokenOffset

	// [CLS]
	ids = append(ids, wp.clsID)
	offsets = append(offsets, TokenOffset{0, 0})

	for wi, word := range words {
		wordStart := wordOffsets[wi]
		subTokens := wp.wordPieceSplit(word)
		pos := wordStart
		for _, sub := range subTokens {
			actual := sub
			if strings.HasPrefix(sub, "##") {
				actual = sub[2:]
			}
			id, ok := wp.vocab[sub]
			if !ok {
				id = wp.unkID
			}
			end := pos + len(actual)
			ids = append(ids, id)
			offsets = append(offsets, TokenOffset{pos, end})
			pos = end
		}
	}

	// [SEP]
	ids = append(ids, wp.sepID)
	offsets = append(offsets, TokenOffset{0, 0})

	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}

	return ids, mask, offsets, nil
}

// tokenizeToWords splits text on whitespace and punctuation,
// returning words and their start byte offsets in the original text.
func tokenizeToWords(lower, original string) ([]string, []int) {
	var words []string
	var starts []int
	runes := []rune(lower)
	origRunes := []rune(original)
	_ = origRunes

	start := -1
	bytePos := 0
	wordByteStart := 0

	for i, r := range runes {
		_ = i
		rLen := len(string(r))
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if start >= 0 {
				words = append(words, string(runes[start:i]))
				starts = append(starts, wordByteStart)
				start = -1
			}
			if unicode.IsPunct(r) {
				// punctuation is its own word
				words = append(words, string(r))
				starts = append(starts, bytePos)
			}
		} else {
			if start < 0 {
				start = i
				wordByteStart = bytePos
			}
		}
		bytePos += rLen
	}
	if start >= 0 {
		words = append(words, string(runes[start:]))
		starts = append(starts, wordByteStart)
	}
	return words, starts
}

// wordPieceSplit applies the WordPiece algorithm to a single word.
func (wp *WordPiece) wordPieceSplit(word string) []string {
	if _, ok := wp.vocab[word]; ok {
		return []string{word}
	}

	runes := []rune(word)
	var tokens []string
	start := 0
	for start < len(runes) {
		end := len(runes)
		found := false
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if _, ok := wp.vocab[sub]; ok {
				tokens = append(tokens, sub)
				start = end
				found = true
				break
			}
			end--
		}
		if !found {
			return []string{"[UNK]"}
		}
	}
	return tokens
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tokenizer/... -v
```

Expected: `PASS` for both `TestEncodeKnownTokens` and `TestEncodeUnknownFallsToUNK`.

- [ ] **Step 5: Commit**

```bash
git add internal/tokenizer/
git commit -m "feat(tokenizer): WordPiece tokenizer with offset tracking"
```

---

## Task 3: Chunker

**Files:**
- Create: `internal/chunker/chunker.go`
- Create: `internal/chunker/chunker_test.go`

The chunker splits long token sequences into overlapping windows of ≤510 tokens (leaving room for [CLS]/[SEP] that the model expects internally). 64-token overlap prevents secrets that span chunk boundaries from being missed.

- [ ] **Step 1: Write the failing tests**

Create `internal/chunker/chunker_test.go`:

```go
package chunker_test

import (
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/chunker"
	"github.com/DungNguyen0209/aibodyguard/internal/tokenizer"
)

func makeTokens(n int) ([]int64, []int64, []tokenizer.TokenOffset) {
	ids := make([]int64, n)
	mask := make([]int64, n)
	offsets := make([]tokenizer.TokenOffset, n)
	for i := range ids {
		ids[i] = int64(i + 10)
		mask[i] = 1
		offsets[i] = tokenizer.TokenOffset{Start: i * 5, End: i*5 + 4}
	}
	return ids, mask, offsets
}

func TestShortSequenceProducesOneChunk(t *testing.T) {
	ids, mask, offsets := makeTokens(100)
	chunks := chunker.Chunk(ids, mask, offsets)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0].TokenIDs) != 100 {
		t.Errorf("expected 100 token IDs in chunk, got %d", len(chunks[0].TokenIDs))
	}
}

func TestLongSequenceProducesMultipleChunks(t *testing.T) {
	// 600 tokens → should require at least 2 chunks
	ids, mask, offsets := makeTokens(600)
	chunks := chunker.Chunk(ids, mask, offsets)
	if len(chunks) < 2 {
		t.Errorf("expected >= 2 chunks for 600 tokens, got %d", len(chunks))
	}
	// Each chunk must not exceed MaxChunkSize
	for i, c := range chunks {
		if len(c.TokenIDs) > chunker.MaxChunkSize {
			t.Errorf("chunk %d has %d tokens, exceeds MaxChunkSize %d", i, len(c.TokenIDs), chunker.MaxChunkSize)
		}
	}
}

func TestOverlapExists(t *testing.T) {
	// 600 tokens → second chunk should start before token 510
	ids, mask, offsets := makeTokens(600)
	chunks := chunker.Chunk(ids, mask, offsets)
	if len(chunks) < 2 {
		t.Skip("not enough chunks to test overlap")
	}
	// The second chunk should overlap with the first by ~Overlap tokens
	// chunk[0] covers [0, 510), chunk[1] starts at 510-64=446
	if chunks[1].TokenIDs[0] == ids[510] {
		t.Errorf("expected overlap: chunk[1] should start before token 510")
	}
}

func TestChunkOffsetsSameLength(t *testing.T) {
	ids, mask, offsets := makeTokens(600)
	chunks := chunker.Chunk(ids, mask, offsets)
	for i, c := range chunks {
		if len(c.TokenIDs) != len(c.Offsets) {
			t.Errorf("chunk %d: TokenIDs len %d != Offsets len %d", i, len(c.TokenIDs), len(c.Offsets))
		}
		if len(c.TokenIDs) != len(c.AttentionMask) {
			t.Errorf("chunk %d: TokenIDs len %d != AttentionMask len %d", i, len(c.TokenIDs), len(c.AttentionMask))
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/chunker/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement the chunker**

Create `internal/chunker/chunker.go`:

```go
package chunker

import "github.com/DungNguyen0209/aibodyguard/internal/tokenizer"

const (
	// MaxChunkSize is the maximum number of tokens per chunk.
	// 512 - 2 reserved for [CLS] and [SEP] added by DistilBERT.
	MaxChunkSize = 510
	// Overlap is the number of tokens shared between consecutive chunks
	// to avoid missing secrets that span chunk boundaries.
	Overlap = 64
)

// Chunk represents one inference-ready window of tokens.
type Chunk struct {
	TokenIDs      []int64
	AttentionMask []int64
	Offsets       []tokenizer.TokenOffset
}

// Chunk splits tokenIDs, attentionMask, and offsets into overlapping windows
// of at most MaxChunkSize tokens. If the total length is <= MaxChunkSize,
// a single chunk is returned. Consecutive chunks overlap by Overlap tokens.
func Chunk(tokenIDs []int64, attentionMask []int64, offsets []tokenizer.TokenOffset) []Chunk {
	n := len(tokenIDs)
	if n <= MaxChunkSize {
		return []Chunk{{
			TokenIDs:      tokenIDs,
			AttentionMask: attentionMask,
			Offsets:       offsets,
		}}
	}

	var chunks []Chunk
	step := MaxChunkSize - Overlap
	for start := 0; start < n; start += step {
		end := start + MaxChunkSize
		if end > n {
			end = n
		}
		chunks = append(chunks, Chunk{
			TokenIDs:      tokenIDs[start:end],
			AttentionMask: attentionMask[start:end],
			Offsets:       offsets[start:end],
		})
		if end == n {
			break
		}
	}
	return chunks
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/chunker/... -v
```

Expected: `PASS` for all 4 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/chunker/
git commit -m "feat(chunker): split token sequences into overlapping 510-token windows"
```

---

## Task 4: Model Cache (Download on First Run)

**Files:**
- Create: `internal/modelcache/cache.go`
- Create: `internal/modelcache/cache_test.go`

Downloads libonnxruntime, model.onnx, and vocab.txt to `~/.cache/aibodyguard/` on first run. Skips download if files already exist.

- [ ] **Step 1: Write the failing tests**

Create `internal/modelcache/cache_test.go`:

```go
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

	// Create stub files
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/modelcache/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement the model cache**

Create `internal/modelcache/cache.go`:

```go
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

// libDownloadURL returns the download URL for the onnxruntime shared library.
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
		name   string
		url    string
		dest   string
	}

	artifacts := []artifact{
		{"vocab.txt", vocabURL, VocabPath(cacheDir)},
		{"model.onnx", modelURL, ModelPath(cacheDir)},
	}

	for _, a := range artifacts {
		if _, err := os.Stat(a.dest); err == nil {
			continue // already cached
		}
		fmt.Printf("  Downloading %-40s ", a.name)
		if err := downloadFile(a.url, a.dest); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("download %s: %w", a.name, err)
		}
		fmt.Println("done")
	}

	// Note: libonnxruntime is distributed as a tgz/zip archive.
	// For now we instruct users to place it manually if auto-download of
	// the archive + extraction is not yet implemented.
	// TODO: implement archive extraction for libonnxruntime.
	libDest := LibPath(cacheDir)
	if _, err := os.Stat(libDest); err != nil {
		fmt.Printf("  ⚠ onnxruntime shared library not found at %s\n", libDest)
		fmt.Printf("    Download from: %s\n", libDownloadURL())
		fmt.Printf("    Extract %s to: %s\n", LibName(), filepath.Dir(libDest))
		return fmt.Errorf("onnxruntime shared library missing: %s", libDest)
	}

	return nil
}

// downloadFile downloads url to destPath, creating a temp file first to avoid
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/modelcache/... -v
```

Expected: `PASS` for all 5 tests (no network calls in tests).

- [ ] **Step 5: Commit**

```bash
git add internal/modelcache/
git commit -m "feat(modelcache): download model artifacts on first run"
```

---

## Task 5: ONNX Runtime Model Wrapper

**Files:**
- Create: `internal/mlmodel/model.go`
- Create: `internal/mlmodel/model_test.go`

Wraps `onnxruntime_go` to load the distilbert ONNX model and run token classification inference.

- [ ] **Step 1: Write the failing tests**

Create `internal/mlmodel/model_test.go`:

```go
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
	// Should return one score per token
	if len(scores) != len(tokenIDs) {
		t.Errorf("expected %d scores, got %d", len(tokenIDs), len(scores))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/mlmodel/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement the model wrapper**

Create `internal/mlmodel/model.go`:

```go
package mlmodel

import (
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

// Model wraps an ONNX Runtime session for the distilbert-secret-masker model.
type Model struct {
	session *ort.DynamicAdvancedSession
}

// New loads the ONNX Runtime shared library and the distilbert model from cacheDir.
// Returns an error if either is missing or fails to load.
func New(cacheDir string) (*Model, error) {
	libPath := modelcache.LibPath(cacheDir)
	modelPath := modelcache.ModelPath(cacheDir)

	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"logits"},
		nil,
	)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("load model: %w", err)
	}

	return &Model{session: session}, nil
}

// Available returns true if the model is loaded and ready for inference.
func (m *Model) Available() bool {
	return m != nil && m.session != nil
}

// Predict runs inference on a single chunk of token IDs.
// Returns a float32 slice of SECRET probability scores, one per input token.
// Scores are derived from the logits via softmax over the two label classes.
func (m *Model) Predict(tokenIDs []int64, attentionMask []int64) ([]float32, error) {
	if !m.Available() {
		return nil, fmt.Errorf("model not available")
	}

	seqLen := int64(len(tokenIDs))
	shape := ort.NewShape(1, seqLen)

	inputIDsTensor, err := ort.NewTensor(shape, tokenIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	maskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	// logits shape: [1, seqLen, numLabels] where numLabels=3 (O, B-SECRET, I-SECRET)
	logitsShape := ort.NewShape(1, seqLen, 3)
	logitsTensor, err := ort.NewEmptyTensor[float32](logitsShape)
	if err != nil {
		return nil, fmt.Errorf("create logits tensor: %w", err)
	}
	defer logitsTensor.Destroy()

	err = m.session.Run(
		[]ort.Value{inputIDsTensor, maskTensor},
		[]ort.Value{logitsTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("inference: %w", err)
	}

	rawLogits := logitsTensor.GetData()
	// rawLogits layout: [token0_O, token0_B-SECRET, token0_I-SECRET, token1_O, ...]
	scores := make([]float32, len(tokenIDs))
	for i := range tokenIDs {
		base := i * 3
		oLogit := float64(rawLogits[base])
		bLogit := float64(rawLogits[base+1])
		iLogit := float64(rawLogits[base+2])
		// softmax over the three logits; combine B-SECRET + I-SECRET probability
		maxL := math.Max(oLogit, math.Max(bLogit, iLogit))
		expO := math.Exp(oLogit - maxL)
		expB := math.Exp(bLogit - maxL)
		expI := math.Exp(iLogit - maxL)
		sum := expO + expB + expI
		scores[i] = float32((expB + expI) / sum)
	}
	return scores, nil
}

// Close releases ONNX Runtime resources.
func (m *Model) Close() {
	if m == nil {
		return
	}
	if m.session != nil {
		m.session.Destroy()
		m.session = nil
	}
	ort.DestroyEnvironment()
}
```

- [ ] **Step 4: Run the unit tests (integration test will be skipped)**

```bash
go test ./internal/mlmodel/... -v
```

Expected: `TestNewModelUnavailableWhenCacheMissing` PASS, `TestAvailableReturnsFalseBeforeInit` PASS, `TestPredictWithRealModel` SKIP (model not cached).

- [ ] **Step 5: Commit**

```bash
git add internal/mlmodel/
git commit -m "feat(mlmodel): ONNX Runtime wrapper for distilbert-secret-masker"
```

---

## Task 6: Detector (Full Pipeline)

**Files:**
- Create: `internal/detector/detector.go`
- Create: `internal/detector/detector_test.go`

Orchestrates: raw file content → tokenize → chunk → infer → extract secret strings.

- [ ] **Step 1: Write the failing tests**

Create `internal/detector/detector_test.go`:

```go
package detector_test

import (
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/detector"
	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

// TestDetectorUnavailableWithNoModel verifies that New() returns an error
// when the model cache is empty, and Available() returns false.
func TestDetectorUnavailableWithNoModel(t *testing.T) {
	d, err := detector.New("/nonexistent/cache/dir")
	if err == nil {
		t.Error("expected error for missing cache dir")
	}
	if d != nil && d.Available() {
		t.Error("detector should not be available with missing model")
	}
}

// TestDetectSecretInPassword verifies the full pipeline detects a password value.
// Skipped if model is not cached.
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

// TestDetectCommentedSecret verifies commented-out secrets are detected.
// Skipped if model is not cached.
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/detector/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement the detector**

Create `internal/detector/detector.go`:

```go
package detector

import (
	"strings"

	"github.com/DungNguyen0209/aibodyguard/internal/chunker"
	"github.com/DungNguyen0209/aibodyguard/internal/mlmodel"
	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
	"github.com/DungNguyen0209/aibodyguard/internal/tokenizer"
)

const secretThreshold = float32(0.80)

// Detector orchestrates the full ML secret detection pipeline.
type Detector struct {
	model *mlmodel.Model
	tok   *tokenizer.WordPiece
}

// New initialises a Detector by loading the ONNX model and vocab from cacheDir.
func New(cacheDir string) (*Detector, error) {
	m, err := mlmodel.New(cacheDir)
	if err != nil {
		return nil, err
	}

	tok, err := tokenizer.NewWordPiece(modelcache.VocabPath(cacheDir))
	if err != nil {
		m.Close()
		return nil, err
	}

	return &Detector{model: m, tok: tok}, nil
}

// Available returns true if the detector is ready to run inference.
func (d *Detector) Available() bool {
	return d != nil && d.model.Available()
}

// Close releases ONNX Runtime resources.
func (d *Detector) Close() {
	if d != nil {
		d.model.Close()
	}
}

// DetectFromContent runs the belt + suspenders detection pipeline on raw file content:
//  1. [Belt]       Feed full raw content through model (including comment lines)
//  2. [Suspenders] Strip comment markers and re-feed each commented line
//
// Returns deduplicated list of detected secret strings.
func (d *Detector) DetectFromContent(content string) ([]string, error) {
	seen := make(map[string]struct{})

	// Belt: full content
	belt, err := d.detectInText(content)
	if err != nil {
		return nil, err
	}
	for _, s := range belt {
		seen[s] = struct{}{}
	}

	// Suspenders: strip comment markers and re-run on each commented line
	for _, line := range strings.Split(content, "\n") {
		stripped := strings.TrimSpace(line)
		if !strings.HasPrefix(stripped, "#") && !strings.HasPrefix(stripped, "!") {
			continue
		}
		cleaned := strings.TrimLeft(stripped, "#!")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}
		susp, err := d.detectInText(cleaned)
		if err != nil {
			continue // best-effort
		}
		for _, s := range susp {
			seen[s] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	return result, nil
}

// detectInText tokenizes text, chunks it, runs inference on each chunk,
// and extracts substrings whose tokens are labeled SECRET (score >= threshold).
func (d *Detector) detectInText(text string) ([]string, error) {
	ids, mask, offsets, err := d.tok.Encode(text)
	if err != nil {
		return nil, err
	}

	chunks := chunker.Chunk(ids, mask, offsets)

	// Track which byte ranges in original text are SECRET to dedup across chunks
	type span struct{ start, end int }
	secretSpans := make(map[span]struct{})

	for _, ch := range chunks {
		scores, err := d.model.Predict(ch.TokenIDs, ch.AttentionMask)
		if err != nil {
			continue // best-effort per chunk
		}

		// Merge consecutive SECRET tokens into spans
		inSecret := false
		var spanStart, spanEnd int
		for i, score := range scores {
			off := ch.Offsets[i]
			if off.Start == 0 && off.End == 0 {
				// CLS or SEP — reset
				if inSecret {
					secretSpans[span{spanStart, spanEnd}] = struct{}{}
					inSecret = false
				}
				continue
			}
			if score >= secretThreshold {
				if !inSecret {
					inSecret = true
					spanStart = off.Start
				}
				spanEnd = off.End
			} else {
				if inSecret {
					secretSpans[span{spanStart, spanEnd}] = struct{}{}
					inSecret = false
				}
			}
		}
		if inSecret {
			secretSpans[span{spanStart, spanEnd}] = struct{}{}
		}
	}

	seen := make(map[string]struct{})
	for sp := range secretSpans {
		if sp.start < 0 || sp.end > len(text) || sp.start >= sp.end {
			continue
		}
		s := text[sp.start:sp.end]
		s = strings.TrimSpace(s)
		if s != "" {
			seen[s] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	return result, nil
}
```

- [ ] **Step 4: Run the unit tests (integration tests skipped without model)**

```bash
go test ./internal/detector/... -v
```

Expected: `TestDetectorUnavailableWithNoModel` PASS, integration tests SKIP.

- [ ] **Step 5: Commit**

```bash
git add internal/detector/
git commit -m "feat(detector): ML pipeline orchestrator with belt+suspenders strategy"
```

---

## Task 7: Wire Detector into DiscoverSecrets

**Files:**
- Modify: `internal/parser/discover.go`
- Modify: `internal/parser/discover_test.go`

Adds the detector to `DiscoverSecrets` as an optional parameter. If `nil` or unavailable, falls back to heuristic only.

- [ ] **Step 1: Update `discover_test.go` to pass `nil` detector**

Read the existing test file first, then add `nil` as second parameter to all `DiscoverSecrets` calls.

```bash
go test ./internal/parser/... -v
```

Expected: compile error after modifying the signature (step 2 below).

- [ ] **Step 2: Update `DiscoverSecrets` signature in `discover.go`**

Change the function signature from:

```go
func DiscoverSecrets(root string) (map[string][]string, error) {
```

to:

```go
func DiscoverSecrets(root string, det *detector.Detector) (map[string][]string, error) {
```

Add import: `"github.com/DungNguyen0209/aibodyguard/internal/detector"`

- [ ] **Step 3: Add ML detection inside the walk function**

Inside `DiscoverSecrets`, after the existing `mergeInto(all, parsed)` call, add:

```go
// ML detection: belt + suspenders
if det != nil && det.Available() {
    raw, readErr := os.ReadFile(path)
    if readErr == nil {
        mlSecrets, mlErr := det.DetectFromContent(string(raw))
        if mlErr == nil {
            for _, s := range mlSecrets {
                if s == "" {
                    continue
                }
                already := false
                for _, existing := range all["_ml"] {
                    if existing == s {
                        already = true
                        break
                    }
                }
                if !already {
                    all["_ml"] = append(all["_ml"], s)
                }
            }
        }
    }
}
```

The `_ml` key is a synthetic bucket — the scanner flattens all values so the key name doesn't matter.

- [ ] **Step 4: Update `fileParser.Discover` to accept and pass detector**

In `parser.go`, update the `Parser` interface and `fileParser.Discover`:

```go
// Parser discovers credential key/value pairs from files rooted at a directory.
type Parser interface {
    Discover(root string, det *detector.Detector) (map[string][]string, error)
}

func (p *fileParser) Discover(root string, det *detector.Detector) (map[string][]string, error) {
    return DiscoverSecrets(root, det)
}
```

- [ ] **Step 5: Run all parser tests to verify they pass**

```bash
go test ./internal/parser/... -v
```

Expected: all existing tests PASS (they pass `nil` for detector).

- [ ] **Step 6: Commit**

```bash
git add internal/parser/
git commit -m "feat(parser): wire ML detector into DiscoverSecrets with heuristic fallback"
```

---

## Task 8: Wire Everything into main.go

**Files:**
- Modify: `cmd/aibodyguard/main.go`

Adds model cache init and detector init to startup. Passes detector into `DiscoverSecrets`. Prints warning if ML model is unavailable.

- [ ] **Step 1: Read current main.go**

```bash
cat cmd/aibodyguard/main.go
```

- [ ] **Step 2: Add cache and detector init to startup**

Find where `parser.New().Discover(root)` is called. Replace with:

```go
import (
    "github.com/DungNguyen0209/aibodyguard/internal/detector"
    "github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

// In startup, before Discover:
cacheDir := modelcache.DefaultCacheDir()
var det *detector.Detector

if err := modelcache.EnsureReady(cacheDir); err != nil {
    fmt.Fprintf(os.Stderr, "  warning: ML model not available, using heuristic detection only\n")
} else {
    det, err = detector.New(cacheDir)
    if err != nil {
        fmt.Fprintf(os.Stderr, "  warning: ML model failed to load (%v), using heuristic detection only\n", err)
        det = nil
    }
}
defer func() {
    if det != nil {
        det.Close()
    }
}()

// Then pass det to Discover:
secrets, err := p.Discover(root, det)
```

- [ ] **Step 3: Build the project**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./... -v
```

Expected: all unit tests PASS, integration tests SKIP (model not cached).

- [ ] **Step 5: Commit**

```bash
git add cmd/aibodyguard/main.go
git commit -m "feat(main): init ML model cache and detector on startup"
```

---

## Task 9: Final Verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -count=1
```

Expected: all tests PASS or SKIP (no FAIL).

- [ ] **Step 2: Build binary**

```bash
go build -o /tmp/aibodyguard-test ./cmd/aibodyguard/
```

Expected: binary produced with no errors.

- [ ] **Step 3: Verify binary runs and falls back gracefully (no model cached)**

```bash
/tmp/aibodyguard-test --version
```

Expected: prints version, no crash.

- [ ] **Step 4: Check git log**

```bash
git log --oneline feature/ml-secret-detection
```

Expected: 8 commits, one per task.

- [ ] **Step 5: Push feature branch**

```bash
git push -u origin feature/ml-secret-detection
```
