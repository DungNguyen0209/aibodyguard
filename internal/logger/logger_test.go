package logger_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DungNguyen0209/aibodyguard/internal/logger"
)

func TestRequestLogger_WritesJSONLine(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "reqlog-*.json")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()

	l, err := logger.New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	entry := logger.RequestEntry{
		Timestamp:    time.Now().UTC(),
		Method:       "POST",
		URL:          "https://api.openai.com/v1/chat/completions",
		Headers:      http.Header{"Content-Type": []string{"application/json"}},
		BodyOriginal: `{"model":"gpt-4"}`,
		BodyRedacted: `{"model":"gpt-4"}`,
		RedactedKeys: []string{},
	}

	if err := l.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() {
		t.Fatal("expected one JSON line, got none")
	}

	var got logger.RequestEntry
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Method != "POST" {
		t.Errorf("method: got %q want %q", got.Method, "POST")
	}
	if got.URL != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("url: got %q", got.URL)
	}
	if got.BodyOriginal != `{"model":"gpt-4"}` {
		t.Errorf("body_original: got %q", got.BodyOriginal)
	}
}

func TestRequestLogger_AppendsAcrossOpen(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "reqlog-*.json")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()

	entry := logger.RequestEntry{
		Timestamp:    time.Now().UTC(),
		Method:       "GET",
		URL:          "https://example.com/",
		Headers:      http.Header{},
		RedactedKeys: []string{},
	}

	// Write once
	l1, _ := logger.New(path)
	_ = l1.Log(entry)
	_ = l1.Close()

	// Write again (append)
	l2, _ := logger.New(path)
	_ = l2.Log(entry)
	_ = l2.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}
