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
	chunks := chunker.Split(ids, mask, offsets)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0].TokenIDs) != 100 {
		t.Errorf("expected 100 token IDs in chunk, got %d", len(chunks[0].TokenIDs))
	}
}

func TestLongSequenceProducesMultipleChunks(t *testing.T) {
	ids, mask, offsets := makeTokens(600)
	chunks := chunker.Split(ids, mask, offsets)
	if len(chunks) < 2 {
		t.Errorf("expected >= 2 chunks for 600 tokens, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.TokenIDs) > chunker.MaxChunkSize {
			t.Errorf("chunk %d has %d tokens, exceeds MaxChunkSize %d", i, len(c.TokenIDs), chunker.MaxChunkSize)
		}
	}
}

func TestOverlapExists(t *testing.T) {
	ids, mask, offsets := makeTokens(600)
	chunks := chunker.Split(ids, mask, offsets)
	if len(chunks) < 2 {
		t.Skip("not enough chunks to test overlap")
	}
	// chunk[1] should start before token 510 (i.e. overlap with chunk[0])
	if chunks[1].TokenIDs[0] == ids[510] {
		t.Errorf("expected overlap: chunk[1] should start before token 510")
	}
}

func TestChunkOffsetsSameLength(t *testing.T) {
	ids, mask, offsets := makeTokens(600)
	chunks := chunker.Split(ids, mask, offsets)
	for i, c := range chunks {
		if len(c.TokenIDs) != len(c.Offsets) {
			t.Errorf("chunk %d: TokenIDs len %d != Offsets len %d", i, len(c.TokenIDs), len(c.Offsets))
		}
		if len(c.TokenIDs) != len(c.AttentionMask) {
			t.Errorf("chunk %d: TokenIDs len %d != AttentionMask len %d", i, len(c.TokenIDs), len(c.AttentionMask))
		}
	}
}
