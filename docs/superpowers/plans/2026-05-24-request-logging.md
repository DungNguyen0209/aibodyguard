# Request Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Log all outgoing HTTPS requests (URL, method, headers, original body, redacted body) intercepted by the MITM proxy as newline-delimited JSON to `/tmp/aibodyguard-requests.log`.

**Architecture:** A new `internal/logger` package provides a `RequestLogger` type that owns the log file handle and encodes `RequestEntry` structs as JSON lines. `mitmProxy` in `internal/mitm/proxy.go` gains a `reqLogger *logger.RequestLogger` field and calls `reqLogger.Log()` after each body redaction. The `mitm.New()` factory creates the logger and injects it.

**Tech Stack:** Go stdlib only — `encoding/json`, `os`, `sync`, `time`.

---

### Task 1: Create `internal/logger` package with `RequestLogger`

**Files:**
- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/logger/logger_test.go`:

```go
package logger_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/yourusername/aibodyguard/internal/logger"
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
		Timestamp: time.Now().UTC(),
		Method:    "GET",
		URL:       "https://example.com/",
		Headers:   http.Header{},
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
```

> Note: add `"strings"` to the imports in the test file.

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/logger/...
```

Expected: `cannot find package "github.com/yourusername/aibodyguard/internal/logger"`

- [ ] **Step 3: Implement `internal/logger/logger.go`**

```go
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
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/logger/... -v
```

Expected: `PASS` for both `TestRequestLogger_WritesJSONLine` and `TestRequestLogger_AppendsAcrossOpen`.

- [ ] **Step 5: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add internal/logger/logger.go internal/logger/logger_test.go
git commit -m "feat: add internal/logger package for request logging"
```

---

### Task 2: Wire `RequestLogger` into `mitmProxy`

**Files:**
- Modify: `internal/mitm/proxy.go`
- Modify: `internal/mitm/mitm.go`

- [ ] **Step 1: Write a failing integration test**

Create `internal/mitm/proxy_logging_test.go`:

```go
package mitm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yourusername/aibodyguard/internal/logger"
	"github.com/yourusername/aibodyguard/internal/scanner"
)

func TestProxyLogsRequest(t *testing.T) {
	// Upstream server that records what it received
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	logFile := t.TempDir() + "/req.log"
	reqLogger, err := logger.New(logFile)
	if err != nil {
		t.Fatal(err)
	}

	sc := scanner.New(map[string]string{"MY_SECRET": "supersecret123"})
	logBuf := &bytes.Buffer{}
	p, err := newMITMProxyWithUpstreamTLSAndLogger(sc, logBuf, upstream.TLS, reqLogger)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Shutdown()

	// Connect through proxy
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	host := strings.TrimPrefix(upstream.URL, "https://")
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", host, host)

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("CONNECT failed: %v %v", err, resp)
	}

	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}

	body := `{"key":"supersecret123"}`
	fmt.Fprintf(tlsConn, "POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\nContent-Type: application/json\r\n\r\n%s", host, len(body), body)

	http.ReadResponse(bufio.NewReader(tlsConn), nil)
	tlsConn.Close()

	reqLogger.Close()

	data, err := os.ReadFile(logFile)
	if err != nil || len(data) == 0 {
		t.Fatalf("log file empty or missing: %v", err)
	}

	var entry logger.RequestEntry
	if err := json.Unmarshal(bytes.TrimRight(data, "\n"), &entry); err != nil {
		t.Fatalf("unmarshal log entry: %v\nraw: %s", err, data)
	}

	if entry.Method != "POST" {
		t.Errorf("method: got %q want POST", entry.Method)
	}
	if entry.BodyOriginal != body {
		t.Errorf("body_original: got %q want %q", entry.BodyOriginal, body)
	}
	if !strings.Contains(entry.BodyRedacted, "[REDACTED:MY_SECRET]") {
		t.Errorf("body_redacted should contain redaction marker, got: %q", entry.BodyRedacted)
	}
	if len(entry.RedactedKeys) == 0 || entry.RedactedKeys[0] != "MY_SECRET" {
		t.Errorf("redacted_keys: got %v", entry.RedactedKeys)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/mitm/... -run TestProxyLogsRequest -v
```

Expected: compile error — `newMITMProxyWithUpstreamTLSAndLogger` undefined.

- [ ] **Step 3: Add `reqLogger` field to `mitmProxy` and new constructor in `proxy.go`**

In `internal/mitm/proxy.go`, update the struct and constructors:

```go
// Add import at top:
import (
	// ... existing imports ...
	"github.com/yourusername/aibodyguard/internal/logger"
)

// Update mitmProxy struct — add reqLogger field:
type mitmProxy struct {
	ca              *ca
	scanner         Redactor
	log             io.Writer
	reqLogger       *logger.RequestLogger // may be nil
	listener        net.Listener
	port            int
	once            sync.Once
	upstreamTLSConf *tls.Config
}

// Update newMITMProxy to call the new 4-arg constructor with nil logger:
func newMITMProxy(s Redactor, log io.Writer) (MITM, error) {
	return newMITMProxyWithUpstreamTLSAndLogger(s, log, nil, nil)
}

// Update newMITMProxyWithUpstreamTLS to call the new constructor:
func newMITMProxyWithUpstreamTLS(s Redactor, log io.Writer, upstreamTLS *tls.Config) (MITM, error) {
	return newMITMProxyWithUpstreamTLSAndLogger(s, log, upstreamTLS, nil)
}

// Add new constructor:
func newMITMProxyWithUpstreamTLSAndLogger(s Redactor, log io.Writer, upstreamTLS *tls.Config, reqLogger *logger.RequestLogger) (MITM, error) {
	authority, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	p := &mitmProxy{
		ca:              authority,
		scanner:         s,
		log:             log,
		reqLogger:       reqLogger,
		listener:        ln,
		port:            ln.Addr().(*net.TCPAddr).Port,
		upstreamTLSConf: upstreamTLS,
	}

	go p.serve()
	return p, nil
}
```

- [ ] **Step 4: Add logging call in `proxyHTTP` after redaction**

In `internal/mitm/proxy.go`, inside `proxyHTTP`, after line 165 (after the `for _, key := range redacted` loop), add:

```go
		// Log the full request entry
		if p.reqLogger != nil {
			entry := logger.RequestEntry{
				Timestamp:    time.Now().UTC(),
				Method:       req.Method,
				URL:          "https://" + hostname + req.URL.RequestURI(),
				Headers:      req.Header.Clone(),
				BodyOriginal: string(bodyBytes),
				BodyRedacted: cleaned,
				RedactedKeys: redacted,
			}
			if redacted == nil {
				entry.RedactedKeys = []string{}
			}
			if err := p.reqLogger.Log(entry); err != nil {
				fmt.Fprintf(p.log, "[aibodyguard] request log write error: %v\n", err)
			}
		}
```

Also add `"time"` to the imports in `proxy.go`.

- [ ] **Step 5: Update `mitm.go` — add `RequestLogPath` to config and create logger in `New`**

Replace the contents of `internal/mitm/mitm.go` with:

```go
package mitm

import (
	"fmt"
	"io"

	"github.com/yourusername/aibodyguard/internal/logger"
)

// MITM is a TLS-intercepting CONNECT proxy.
type MITM interface {
	Port() int
	CACertPEM() []byte
	Shutdown()
}

// Config holds options for creating a MITM proxy.
type Config struct {
	// RequestLogPath is the file path for the JSON request log.
	// Defaults to /tmp/aibodyguard-requests.log if empty.
	RequestLogPath string
}

// New creates and starts a MITM proxy. s redacts secrets from request bodies.
// log receives diagnostic lines. cfg may be nil to use defaults.
func New(s Redactor, log io.Writer, cfg *Config) (MITM, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	path := cfg.RequestLogPath
	if path == "" {
		path = "/tmp/aibodyguard-requests.log"
	}

	reqLogger, err := logger.New(path)
	if err != nil {
		fmt.Fprintf(log, "[aibodyguard] WARNING: could not open request log %s: %v\n", path, err)
		reqLogger = nil
	}

	return newMITMProxyWithUpstreamTLSAndLogger(s, log, nil, reqLogger)
}

// Redactor redacts known secret values from text.
type Redactor interface {
	Redact(input string) (cleaned string, redactedKeys []string)
}
```

- [ ] **Step 6: Fix `main.go` to pass the updated `New` signature**

In `cmd/aibodyguard/main.go`, find the call to `mitm.New(...)` and update it to pass a config:

```go
proxy, err := mitm.New(sc, logFile, &mitm.Config{})
```

(If `main.go` currently calls `mitm.New(sc, logFile)` with two args, add the third arg `&mitm.Config{}`.)

- [ ] **Step 7: Fix any existing tests that call `mitm.New` with the old 2-arg signature**

Search for callers:

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
grep -rn "mitm\.New(" --include="*.go"
```

Update each call site to pass a third `*mitm.Config` argument. Pass `nil` to use defaults.

- [ ] **Step 8: Run all tests**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./... -v
```

Expected: all tests pass including `TestProxyLogsRequest`.

- [ ] **Step 9: Build the binary to confirm no compile errors**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build ./cmd/aibodyguard/...
```

Expected: no output (success).

- [ ] **Step 10: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add internal/mitm/proxy.go internal/mitm/mitm.go internal/mitm/proxy_logging_test.go cmd/aibodyguard/main.go
git commit -m "feat: wire request logger into mitmProxy"
```

---

### Task 3: Manual smoke test

- [ ] **Step 1: Run the proxy with a test agent command**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
./aibodyguard -- node /tmp/test-proxy.ts
```

- [ ] **Step 2: Inspect the request log**

```bash
cat /tmp/aibodyguard-requests.log
```

Expected: one JSON object per line, each containing `timestamp`, `method`, `url`, `headers`, `body_original`, `body_redacted`, `redacted_keys`.

- [ ] **Step 3: Pretty-print a single entry to verify structure**

```bash
head -1 /tmp/aibodyguard-requests.log | python3 -m json.tool
```

Expected: valid, human-readable JSON with all required fields present.
