package tokenizer_test

import (
	"os"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/tokenizer"
)

// TestEncodeKnownTokens uses a minimal in-memory vocab to verify encoding
// without requiring the real vocab.txt file.
func TestEncodeKnownTokens(t *testing.T) {
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
