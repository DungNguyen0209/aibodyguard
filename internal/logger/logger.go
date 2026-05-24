package logger

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

// RequestEntry holds the data logged for each intercepted request.
type RequestEntry struct {
	Timestamp    time.Time   `json:"timestamp"`
	Method       string      `json:"method"`
	URL          string      `json:"url"`
	Headers      http.Header `json:"headers"`
	BodyOriginal string      `json:"body_original"`
	BodyRedacted string      `json:"body_redacted"`
	RedactedKeys []string    `json:"redacted_keys"`
}

// RequestLogger writes RequestEntry values as newline-delimited JSON.
// It is safe for concurrent use.
type RequestLogger struct {
	mu  sync.Mutex
	enc *json.Encoder
	f   *os.File
}

// New opens (or creates) the file at path in append mode and returns a RequestLogger.
func New(path string) (*RequestLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &RequestLogger{
		enc: json.NewEncoder(f),
		f:   f,
	}, nil
}

// Log encodes one RequestEntry as a JSON line.
func (l *RequestLogger) Log(e RequestEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enc.Encode(e)
}

// Close closes the underlying file.
func (l *RequestLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
