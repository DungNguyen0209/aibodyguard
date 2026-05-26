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
	vocab map[string]int64
	unkID int64
	clsID int64
	sepID int64
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
func tokenizeToWords(lower, _ string) ([]string, []int) {
	var words []string
	var starts []int

	runes := []rune(lower)
	start := -1
	bytePos := 0
	wordByteStart := 0

	for i, r := range runes {
		rLen := len(string(r))
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if start >= 0 {
				words = append(words, string(runes[start:i]))
				starts = append(starts, wordByteStart)
				start = -1
			}
			if unicode.IsPunct(r) {
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
