# Project Layout & Interface Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganise AIBodyguard to follow golang-standards/project-layout and introduce explicit Go interfaces for all three internal packages so `main.go` depends only on interfaces, not concrete types.

**Architecture:** Each internal package exposes a single interface (`Parser`, `Scanner`, `Proxy`) defined in its own file alongside an unexported concrete implementation. Constructors return the interface type. `main.go` is wired exclusively via these interfaces. A `Makefile` and `scripts/build.sh` are added for the golang-standards `build/` and `scripts/` conventions.

**Tech Stack:** Go 1.22+, standard library only (plus `gopkg.in/yaml.v3`).

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `internal/scanner/scanner.go` | Modify | Add `Scanner` interface; rename struct to `redactScanner`; `New` returns `Scanner` |
| `internal/parser/parser.go` | Create | `Parser` interface + `New() Parser` constructor |
| `internal/parser/discover.go` | Modify | Rename `DiscoverSecrets` → method on `fileParser`; keep all helpers |
| `internal/proxy/proxy.go` | Modify | Add `Proxy` interface; rename struct to `httpProxy`; `New` returns `Proxy`; accept `scanner.Scanner` interface |
| `internal/scanner/scanner_test.go` | Modify | Call via `Scanner` interface (no concrete type in test) |
| `internal/parser/discover_test.go` | Modify | Call via `Parser` interface |
| `cmd/aibodyguard/main.go` | Modify | Use `parser.New().Discover()` instead of `parser.DiscoverSecrets()`; types are interfaces |
| `Makefile` | Create | `build`, `test`, `lint`, `clean` targets |
| `scripts/build.sh` | Create | Local build helper script |
| `build/ci/` | Create | Empty dir with `.gitkeep` (CI stays in `.github/`) |

---

## Task 1: Add `Scanner` interface to `internal/scanner`

**Files:**
- Modify: `internal/scanner/scanner.go`
- Modify: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write the failing test that uses the interface type**

Replace `internal/scanner/scanner_test.go` with:

```go
package scanner

import (
	"strings"
	"testing"
)

func TestScanAndRedact(t *testing.T) {
	secrets := map[string]string{
		"DB_PASSWORD":  "supersecret123",
		"API_KEY":      "sk-abc123xyz456",
		"database.url": "postgres://supersecret123@localhost/db",
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"my password is supersecret123 and key sk-abc123xyz456"}]}`
	result, redacted := s.Redact(body)

	if strings.Contains(result, "supersecret123") {
		t.Error("supersecret123 should be redacted")
	}
	if strings.Contains(result, "sk-abc123xyz456") {
		t.Error("sk-abc123xyz456 should be redacted")
	}
	if !strings.Contains(result, "[REDACTED:DB_PASSWORD]") {
		t.Error("should contain [REDACTED:DB_PASSWORD]")
	}
	if !strings.Contains(result, "[REDACTED:API_KEY]") {
		t.Error("should contain [REDACTED:API_KEY]")
	}

	redactedSet := make(map[string]bool)
	for _, k := range redacted {
		redactedSet[k] = true
	}
	if !redactedSet["DB_PASSWORD"] {
		t.Error("DB_PASSWORD should be in redacted list")
	}
	if !redactedSet["API_KEY"] {
		t.Error("API_KEY should be in redacted list")
	}
	if len(redacted) != 2 {
		t.Errorf("expected 2 redacted keys, got %d: %v", len(redacted), redacted)
	}
}

func TestScanAndRedactNoMatch(t *testing.T) {
	secrets := map[string]string{
		"DB_PASSWORD": "supersecret123",
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"nothing secret here"}]}`
	result, redacted := s.Redact(body)

	if result != body {
		t.Error("body should be unchanged when no secrets found")
	}
	if len(redacted) != 0 {
		t.Errorf("expected 0 redacted keys, got %d", len(redacted))
	}
}
```

- [ ] **Step 2: Run test — expect compile failure because `Scanner` is a struct, not an interface**

```bash
export PATH="/opt/homebrew/bin:$PATH"
go test ./internal/scanner/...
```

Expected: `cannot use New(secrets) (value of type *Scanner) as type Scanner`

- [ ] **Step 3: Update `internal/scanner/scanner.go` — add interface, rename struct**

Replace the entire file with:

```go
package scanner

import (
	"sort"
	"strings"
)

// Scanner redacts known secret values from arbitrary text.
type Scanner interface {
	Redact(input string) (cleaned string, redactedKeys []string)
}

// New returns a Scanner loaded with the given secrets map.
func New(secrets map[string]string) Scanner {
	return &redactScanner{secrets: secrets}
}

// redactScanner is the concrete implementation of Scanner.
type redactScanner struct {
	secrets map[string]string
}

// entry is a key-value pair used for sorted processing.
type entry struct {
	key string
	val string
}

// Redact scans body for any known secret values and replaces them with
// [REDACTED:<KEY_NAME>]. Secrets are matched longest-value-first to prevent
// substring collisions. Returns the (possibly modified) body and the sorted
// list of redacted key names.
func (s *redactScanner) Redact(body string) (string, []string) {
	entries := make([]entry, 0, len(s.secrets))
	for k, v := range s.secrets {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].val) > len(entries[j].val)
	})

	var redactedKeys []string
	result := body

	for _, e := range entries {
		if strings.Contains(result, e.val) {
			placeholder := "[REDACTED:" + e.key + "]"
			result = strings.ReplaceAll(result, e.val, placeholder)
			redactedKeys = append(redactedKeys, e.key)
		}
	}

	sort.Strings(redactedKeys)
	return result, redactedKeys
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
export PATH="/opt/homebrew/bin:$PATH"
go test ./internal/scanner/...
```

Expected: `ok  github.com/yourusername/aibodyguard/internal/scanner`

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/AIBodyguard
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "refactor(scanner): extract Scanner interface, unexport concrete type"
```

---

## Task 2: Add `Parser` interface to `internal/parser`

**Files:**
- Create: `internal/parser/parser.go`
- Modify: `internal/parser/discover.go`
- Modify: `internal/parser/discover_test.go`

- [ ] **Step 1: Read the current discover_test.go to understand what needs updating**

```bash
cat ~/Documents/AIBodyguard/internal/parser/discover_test.go
```

- [ ] **Step 2: Create `internal/parser/parser.go` — interface + constructor**

```go
package parser

// Parser discovers credential key/value pairs from files rooted at a directory.
type Parser interface {
	Discover(root string) (map[string]string, error)
}

// New returns a Parser backed by the filesystem implementation.
func New() Parser {
	return &fileParser{}
}
```

- [ ] **Step 3: Add `fileParser` struct and `Discover` method to `internal/parser/discover.go`**

At the top of `discover.go`, after the `package parser` line and imports, add:

```go
// fileParser is the concrete implementation of Parser.
type fileParser struct{}

// Discover walks root recursively, parses all credential files,
// and returns a merged map of key -> secret value.
func (p *fileParser) Discover(root string) (map[string]string, error) {
	return DiscoverSecrets(root)
}
```

Keep `DiscoverSecrets` as-is for now (it is still used internally). The method delegates to it.

- [ ] **Step 4: Update `discover_test.go` to use the interface**

Read the current test file first (`cat internal/parser/discover_test.go`), then replace any direct calls to `DiscoverSecrets(root)` with `New().Discover(root)`. For example, if the test has:

```go
secrets, err := DiscoverSecrets(dir)
```

Change to:

```go
secrets, err := parser.New().Discover(dir)
```

or, since we're inside the `parser` package (same package test):

```go
secrets, err := New().Discover(dir)
```

- [ ] **Step 5: Run parser tests — expect pass**

```bash
export PATH="/opt/homebrew/bin:$PATH"
go test ./internal/parser/...
```

Expected: `ok  github.com/yourusername/aibodyguard/internal/parser`

- [ ] **Step 6: Commit**

```bash
cd ~/Documents/AIBodyguard
git add internal/parser/parser.go internal/parser/discover.go internal/parser/discover_test.go
git commit -m "refactor(parser): extract Parser interface with Discover method"
```

---

## Task 3: Add `Proxy` interface to `internal/proxy`

**Files:**
- Modify: `internal/proxy/proxy.go`

- [ ] **Step 1: Update `internal/proxy/proxy.go` — add interface, rename struct, accept `scanner.Scanner` interface**

Replace the entire file with:

```go
package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/yourusername/aibodyguard/internal/scanner"
)

// prefixTargets maps URL path prefixes to real API base URLs.
var prefixTargets = map[string]string{
	"/anthropic": "https://api.anthropic.com",
	"/openai":    "https://api.openai.com",
	"/copilot":   "https://api.githubcopilot.com",
}

// httpSharedClient is a shared HTTP client that reuses TCP connections.
var httpSharedClient = &http.Client{}

// Proxy is a local HTTP interception proxy.
type Proxy interface {
	Port() int
	Shutdown()
}

// New starts a proxy and returns it.
// s is used to redact secrets; log receives redaction event lines.
func New(s scanner.Scanner, log io.Writer) (Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not bind port: %w", err)
	}

	p := &httpProxy{
		scanner: s,
		port:    listener.Addr().(*net.TCPAddr).Port,
		log:     log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handle)
	p.server = &http.Server{Handler: mux}

	go p.server.Serve(listener)
	return p, nil
}

// httpProxy is the concrete implementation of Proxy.
type httpProxy struct {
	scanner scanner.Scanner
	server  *http.Server
	port    int
	log     io.Writer
}

// Port returns the port the proxy is listening on.
func (p *httpProxy) Port() int {
	return p.port
}

// Shutdown stops the proxy server.
func (p *httpProxy) Shutdown() {
	p.server.Close()
}

func (p *httpProxy) handle(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}

	cleaned, redacted := p.scanner.Redact(string(bodyBytes))
	for _, key := range redacted {
		fmt.Fprintf(p.log, "[aibodyguard] redacted: %s\n", key)
	}

	targetBase, upstreamPath := resolveTarget(r)
	upstreamURL := targetBase + upstreamPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewBufferString(cleaned))
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	for key, vals := range r.Header {
		for _, v := range vals {
			upstreamReq.Header.Add(key, v)
		}
	}
	upstreamReq.ContentLength = int64(len(cleaned))

	resp, err := httpSharedClient.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}

// resolveTarget strips the routing prefix and returns the real API base URL and path.
// Falls back to Anthropic for unknown prefixes.
func resolveTarget(r *http.Request) (targetBase, upstreamPath string) {
	path := r.URL.Path
	for prefix, base := range prefixTargets {
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			remainder := path[len(prefix):]
			if remainder == "" {
				remainder = "/"
			}
			return base, remainder
		}
	}
	return "https://api.anthropic.com", path
}
```

- [ ] **Step 2: Build to verify the package compiles**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go build ./internal/proxy/...
```

Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/proxy/proxy.go
git commit -m "refactor(proxy): extract Proxy interface, unexport httpProxy, accept scanner.Scanner interface"
```

---

## Task 4: Update `main.go` to use interfaces throughout

**Files:**
- Modify: `cmd/aibodyguard/main.go`

- [ ] **Step 1: Update the proxy.New call — it now returns `(proxy.Proxy, error)` not `(*proxy.Proxy, error)`**

In `main.go`, find:

```go
p, err := proxy.New(s, logWriter)
```

The variable `p` is already used only via `p.Port()` and `p.Shutdown()` — both are on the interface — so no other changes are needed in the call site. However, `s` is currently `*scanner.Scanner` (old type). After Task 1, `s` is already a `scanner.Scanner` interface value returned by `scanner.New()`. Verify the wiring compiles:

```go
// These lines in main.go should already read as:
s := scanner.New(secrets)          // returns scanner.Scanner
p, err := proxy.New(s, logWriter)  // accepts scanner.Scanner, returns proxy.Proxy
```

- [ ] **Step 2: Update the parser call — use `parser.New().Discover(cwd)` instead of `parser.DiscoverSecrets(cwd)`**

In `main.go`, find:

```go
secrets, err := parser.DiscoverSecrets(cwd)
```

Replace with:

```go
secrets, err := parser.New().Discover(cwd)
```

- [ ] **Step 3: Build the full binary**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go build ./...
```

Expected: no output (success). Binary `aibodyguard` is produced.

- [ ] **Step 4: Smoke test**

```bash
./aibodyguard -- echo ok
```

Expected output includes the startup banner, then `ok`, then the exit line.

- [ ] **Step 5: Run all tests**

```bash
export PATH="/opt/homebrew/bin:$PATH"
go test ./...
```

Expected: all packages pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/aibodyguard/main.go
git commit -m "refactor(main): wire all dependencies via interfaces"
```

---

## Task 5: Add Makefile and scripts (golang-standards layout)

**Files:**
- Create: `Makefile`
- Create: `scripts/build.sh`
- Create: `build/ci/.gitkeep`

- [ ] **Step 1: Create `Makefile`**

```makefile
BINARY  := aibodyguard
CMD     := ./cmd/aibodyguard

.PHONY: build test lint clean

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	staticcheck ./...

clean:
	rm -f $(BINARY)
```

- [ ] **Step 2: Create `scripts/build.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

# Local build helper — mirrors the Makefile build target.
# Usage: ./scripts/build.sh [output-name]
BINARY="${1:-aibodyguard}"
go build -o "$BINARY" ./cmd/aibodyguard
echo "Built: $BINARY"
```

Make it executable:

```bash
chmod +x scripts/build.sh
```

- [ ] **Step 3: Create `build/ci/.gitkeep`**

```bash
mkdir -p build/ci
touch build/ci/.gitkeep
```

- [ ] **Step 4: Verify `make build` and `make test` work**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
make build
make test
```

Expected:
- `make build` produces `./aibodyguard` binary
- `make test` shows all packages passing

- [ ] **Step 5: Commit**

```bash
git add Makefile scripts/build.sh build/ci/.gitkeep
git commit -m "build: add Makefile, scripts/build.sh, build/ci dir (golang-standards layout)"
```

---

## Task 6: Final verification

- [ ] **Step 1: Run full build and test suite from clean state**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
make clean
make build
make test
```

Expected:
- `make clean` removes binary
- `make build` succeeds
- `make test`: all packages `ok`

- [ ] **Step 2: Confirm `main.go` imports no concrete types from internal packages**

```bash
grep -n "scanner\.\|proxy\.\|parser\." cmd/aibodyguard/main.go
```

Confirm none of the matched lines reference `*scanner.Scanner`, `*proxy.Proxy`, or `*parser.fileParser`. All usages should be `scanner.New(...)`, `proxy.New(...)`, `parser.New().Discover(...)` — interface values only.

- [ ] **Step 3: Commit final state**

```bash
git add -A
git status
# If any stray files exist, review before committing
git commit -m "chore: final verification — project layout and interface refactor complete" --allow-empty
```
