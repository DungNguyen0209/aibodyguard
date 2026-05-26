# ML-Based Secret Detection Design

**Date:** 2026-05-26
**Branch:** `feature/ml-secret-detection`

## Goal

Replace the brittle `isLikelySecret` heuristic as the primary secret detection method with the `distilbert-secret-masker` ONNX model, using a "belt + suspenders" approach:

- **Belt:** ML model classifies secrets from raw file content (including commented lines)
- **Suspenders:** ML model re-processes stripped commented lines without `#`/`!` markers
- **Fallback:** Existing `isLikelySecret` heuristic runs on all parsed values, catches anything the model misses

The union of all three feeds the redaction set.

## Runtime Dependencies

| Artifact | Source | Size | Cache Location |
|---|---|---|---|
| `libonnxruntime` (platform-specific) | github.com/microsoft/onnxruntime/releases v1.25.0 | ~30MB | `~/.cache/aibodyguard/lib/` |
| `model.onnx` | `https://huggingface.co/AndrewAndrewsen/distilbert-secret-masker/resolve/main/model.onnx` | ~265MB | `~/.cache/aibodyguard/models/` |
| `vocab.txt` | `https://huggingface.co/AndrewAndrewsen/distilbert-secret-masker/resolve/main/vocab.txt` | ~230KB | `~/.cache/aibodyguard/models/` |

Downloaded automatically on first run. If offline or unavailable, falls back to heuristic-only with a warning.

## Go Dependencies

| Package | Purpose |
|---|---|
| `github.com/yalue/onnxruntime_go` | ONNX Runtime CGo bindings (wraps Microsoft's official ONNX Runtime) |

## New Internal Packages

### `internal/tokenizer`

WordPiece tokenizer in Go.

- `NewWordPiece(vocabPath string) (*WordPiece, error)` — loads vocab.txt
- `Encode(text string) ([]int64, []int64, []TokenOffset, error)` — returns (tokenIDs, attentionMask, offsets)
- `TokenOffset` struct: `{Start, End int}` — maps token back to character positions in original text

### `internal/chunker`

Splits long token sequences into overlapping windows safe for DistilBERT.

- `Chunk(tokenIDs []int64, attentionMask []int64, offsets []TokenOffset) []Chunk`
- Max tokens per chunk: **510** (leaves room for [CLS] / [SEP] special tokens)
- Overlap: **64 tokens** (catches secrets that span chunk boundaries)
- `Chunk` struct: `{TokenIDs []int64, AttentionMask []int64, Offsets []TokenOffset}`

### `internal/mlmodel`

ONNX Runtime session lifecycle and inference.

- `New(cacheDir string) (*Model, error)` — loads shared lib + model.onnx, creates session
- `Predict(tokenIDs []int64, attentionMask []int64) ([]float32, error)` — returns raw logits per token
- `Available() bool` — whether model is loaded and ready
- `Close()` — releases ONNX Runtime resources

### `internal/detector`

Orchestrates the full pipeline: file content → detected secret strings.

- `New(cacheDir string) (*Detector, error)`
- `DetectFromContent(rawContent string) ([]string, error)` — full ML pipeline:
  1. Tokenize raw content
  2. Chunk into ≤510-token windows
  3. Run inference per chunk
  4. Merge SECRET labels (dedup overlapping chunks)
  5. Map token spans back to character substrings
  6. Return list of detected secret strings
- `Available() bool` — delegates to mlmodel.Available()

### `internal/modelcache`

Downloads and caches model artifacts on first run.

- `EnsureReady(cacheDir string) error` — checks cache, downloads missing artifacts with progress bar
- Downloads: `libonnxruntime` (platform-specific URL), `model.onnx`, `vocab.txt`

## Modified Existing Files

### `internal/parser/discover.go`

- `DiscoverSecrets(root string)` gains a `detector *detector.Detector` parameter
- After parsing each file, runs `detector.DetectFromContent(rawContent)` (belt)
- Also strips comment markers from commented lines and re-runs detector (suspenders)
- Unions ML-detected secrets with heuristic-detected secrets (fallback)
- If `detector == nil` or `!detector.Available()`, silently uses heuristic only

### `cmd/aibodyguard/main.go`

- Init `modelcache.EnsureReady(cacheDir)` at startup (downloads if needed)
- Init `detector.New(cacheDir)`
- Pass detector into `DiscoverSecrets`
- Print warning if detector unavailable

## Belt + Suspenders Detail

```
For each credential file:

  [BELT] Feed raw file content (with # comment lines) through detector
    → model sees full context including key names and comment markers

  [SUSPENDERS] For each commented-out line:
    → strip # or ! prefix
    → feed "KEY=VALUE" text through detector
    → ensures model sees clean context even if # confused it

  [FALLBACK] Run existing ParseEnvFile + isLikelySecret on all parsed values
    → catches short secrets the model scores below τ

  Union all three → redaction set
```

## Confidence Threshold

- τ = **0.80** (balanced — default recommended by model card)
- Token is labeled SECRET if softmax score >= 0.80
- Consecutive B-SECRET / I-SECRET tokens are merged into one secret string

## First-Run UX

```
$ aibodyguard claude

  Downloading ML model (first run only)...
    onnxruntime v1.25.0 (darwin-arm64)   ████████ 30MB  done
    distilbert-secret-masker.onnx        ████████ 265MB done
    vocab.txt                            ████████ 230KB done
  Cached at ~/.cache/aibodyguard/

  AIBodyguard v0.2.0  active
  Secrets loaded: 4 values (ML + heuristic)
  ...
```

## Offline / Fallback UX

```
$ aibodyguard claude   # model not cached, no internet

  warning: ML model not available, using heuristic detection only
  AIBodyguard v0.2.0  active
  ...
```

## Testing Requirements

- Unit tests for tokenizer: known inputs → expected token IDs
- Unit tests for chunker: correct chunk boundaries, overlap, dedup
- Unit tests for detector: `POSTGRES_PASSWORD=wb2000` → `wb2000` detected as SECRET
- Unit tests for detector: `POSTGRES_PORT=5432` → `5432` NOT detected as SECRET
- Unit tests for detector: `# OLD_API_KEY=sk-proj-abc123` → `sk-proj-abc123` detected
- Integration test: full pipeline on a real `.env` file

## Limitations

- First run requires ~295MB download
- CGo required (cross-compilation needs platform-specific onnxruntime shared lib)
- 512 token limit per chunk — handled by chunking with overlap
- English-only model
- ~50-100ms added to startup per credential file (parallelisable in future)
