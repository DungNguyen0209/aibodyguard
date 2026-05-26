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
// Returns a deduplicated list of detected secret strings.
func (d *Detector) DetectFromContent(content string) ([]string, error) {
	seen := make(map[string]struct{})

	// Belt: full raw content (model sees key names + values including comments)
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
			continue // best-effort per line
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

// span is a byte range in the original text.
type span struct{ start, end int }

// isWordBoundary returns true for characters that delimit secret "words":
// whitespace, newline, tab, and assignment/quoting characters.
func isWordBoundary(c rune) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '=', '"', '\'', ',', ';', '(', ')', '{', '}', '[', ']':
		return true
	}
	return false
}

// detectInText tokenizes text, chunks it, runs inference on each chunk,
// and extracts substrings whose tokens are labeled SECRET (score >= threshold).
func (d *Detector) detectInText(text string) ([]string, error) {
	ids, mask, offsets, err := d.tok.Encode(text)
	if err != nil {
		return nil, err
	}

	chunks := chunker.Split(ids, mask, offsets)

	secretSpans := make(map[span]struct{})

	for _, ch := range chunks {
		scores, err := d.model.Predict(ch.TokenIDs, ch.AttentionMask)
		if err != nil {
			continue // best-effort per chunk
		}

		inSecret := false
		var spanStart, spanEnd int

		for i, score := range scores {
			off := ch.Offsets[i]
			// CLS and SEP have zero offsets — they are not real text tokens
			if off.Start == 0 && off.End == 0 {
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

	// Word-level promotion: expand each detected span to the boundaries of the
	// full "word" (non-whitespace run) it falls within. This compensates for
	// sub-word tokenisation splitting secret tokens (e.g. "wb" + "2000" or
	// "sk" + "-" + "proj" + "-" + "abc…").
	seen := make(map[string]struct{})
	for sp := range secretSpans {
		if sp.start < 0 || sp.end > len(text) || sp.start >= sp.end {
			continue
		}
		// Expand left to start of word (non-whitespace, non-assignment run)
		wStart := sp.start
		for wStart > 0 && !isWordBoundary(rune(text[wStart-1])) {
			wStart--
		}
		// Expand right to end of word
		wEnd := sp.end
		for wEnd < len(text) && !isWordBoundary(rune(text[wEnd])) {
			wEnd++
		}
		s := strings.TrimSpace(text[wStart:wEnd])
		// Strip leading KEY= prefix: if an assignment operator is present,
		// take only the value part (everything after the last '=').
		if idx := strings.LastIndex(s, "="); idx >= 0 {
			s = s[idx+1:]
		}
		// Strip surrounding quotes/whitespace
		s = strings.Trim(s, `"' `)
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
