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

// Split splits tokenIDs, attentionMask, and offsets into overlapping windows
// of at most MaxChunkSize tokens. If the total length is <= MaxChunkSize,
// a single chunk is returned. Consecutive chunks overlap by Overlap tokens.
func Split(tokenIDs []int64, attentionMask []int64, offsets []tokenizer.TokenOffset) []Chunk {
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
