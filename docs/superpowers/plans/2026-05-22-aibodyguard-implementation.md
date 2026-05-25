# AIBodyguard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI tool that wraps AI coding agents and redacts credential values from outbound LLM API requests.

**Architecture:** CLI wrapper spawns the agent as a child process after injecting `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL` env vars pointing to a local HTTP proxy. The proxy intercepts requests, scans for secret values loaded from auto-discovered credential files, redacts matches as `[REDACTED:<KEY_NAME>]`, then forwards the clean request to the real API.

**Tech Stack:** Go 1.22+, stdlib only (no external dependencies for core logic), `gopkg.in/yaml.v3` for YAML parsing.

---

## File Map

| File | Responsibility |
|------|---------------|
| `cmd/aibodyguard/main.go` | CLI entrypoint: parse args, start proxy, inject env, spawn agent, wait for exit |
| `internal/parser/env.go` | Parse `.env` and `.properties` files into `map[string]string` |
| `internal/parser/json.go` | Parse JSON files, flatten to `map[string]string` |
| `internal/parser/yaml.go` | Parse YAML files, flatten to `map[string]string` |
| `internal/parser/discover.go` | Walk CWD recursively, collect credential files, call parsers |
| `internal/scanner/scanner.go` | Load secrets map, scan string for values, return redacted string + log |
| `internal/proxy/proxy.go` | Start HTTP server, intercept requests, call scanner, forward to real API |
| `go.mod` | Module definition |
| `README.md` | Installation and usage docs |

---

## Task 1: Initialize Go module and project structure

**Files:**
- Create: `go.mod`
- Create: `go.sum` (generated)

- [ ] **Step 1: Create the project directory and initialize Go module**

```bash
cd ~/Documents/AIBodyguard
git init
go mod init github.com/yourusername/aibodyguard
```

- [ ] **Step 2: Add yaml dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p cmd/aibodyguard
mkdir -p internal/parser
mkdir -p internal/scanner
mkdir -p internal/proxy
```

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize Go module"
```

---

## Task 2: .env and .properties parser

**Files:**
- Create: `internal/parser/env.go`
- Create: `internal/parser/env_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/parser/env_test.go`:

```go
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	content := `
# comment line
APP_NAME=myapp
DB_PASSWORD=supersecret123
QUOTED_VAL="quoted value here"
SINGLE_QUOTED='single quoted'
EMPTY_VAL=
! another comment
JAVA_PROP=somevalue
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, ".env")
	os.WriteFile(f, []byte(content), 0644)

	got, err := ParseEnvFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"APP_NAME":     "myapp",
		"DB_PASSWORD":  "supersecret123",
		"QUOTED_VAL":   "quoted value here",
		"SINGLE_QUOTED": "single quoted",
		"JAVA_PROP":    "somevalue",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %s: got %q, want %q", k, got[k], want)
		}
	}
	if _, ok := got["EMPTY_VAL"]; ok {
		t.Error("empty value should be excluded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/Documents/AIBodyguard
go test ./internal/parser/... -run TestParseEnvFile -v
```

Expected: compile error — `ParseEnvFile` not defined.

- [ ] **Step 3: Implement env.go**

Create `internal/parser/env.go`:

```go
package parser

import (
	"bufio"
	"os"
	"strings"
)

// ParseEnvFile parses a .env or .properties file and returns key=value pairs.
// Lines starting with # or ! are treated as comments and skipped.
// Values are unquoted (strips surrounding " or ').
// Empty values are excluded.
func ParseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" || val == "" {
			continue
		}
		result[key] = val
	}
	return result, scanner.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/parser/... -run TestParseEnvFile -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/env.go internal/parser/env_test.go
git commit -m "feat: add .env/.properties parser"
```

---

## Task 3: JSON parser

**Files:**
- Create: `internal/parser/json.go`
- Create: `internal/parser/json_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/parser/json_test.go`:

```go
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONFile(t *testing.T) {
	content := `{
  "database": {
    "password": "db-secret-password",
    "host": "localhost"
  },
  "api_key": "sk-abc123xyz456",
  "port": 5432,
  "enabled": true
}`
	tmp := t.TempDir()
	f := filepath.Join(tmp, "credentials.json")
	os.WriteFile(f, []byte(content), 0644)

	got, err := ParseJSONFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"database.password": "db-secret-password",
		"database.host":     "localhost",
		"api_key":           "sk-abc123xyz456",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %s: got %q, want %q", k, got[k], want)
		}
	}
	// non-string values should not be present
	if _, ok := got["port"]; ok {
		t.Error("numeric value should be excluded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/parser/... -run TestParseJSONFile -v
```

Expected: compile error — `ParseJSONFile` not defined.

- [ ] **Step 3: Implement json.go**

Create `internal/parser/json.go`:

```go
package parser

import (
	"encoding/json"
	"os"
	"strings"
)

// ParseJSONFile parses a JSON file and returns flattened key=value pairs.
// Nested keys are dot-separated (e.g., "database.password").
// Only string leaf values are included.
func ParseJSONFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	flattenJSON("", raw, result)
	return result, nil
}

func flattenJSON(prefix string, v interface{}, out map[string]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenJSON(key, child, out)
		}
	case []interface{}:
		for i, child := range val {
			key := strings.Join([]string{prefix, string(rune('0' + i))}, ".")
			flattenJSON(key, child, out)
		}
	case string:
		if prefix != "" && val != "" {
			out[prefix] = val
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/parser/... -run TestParseJSONFile -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/json.go internal/parser/json_test.go
git commit -m "feat: add JSON file parser"
```

---

## Task 4: YAML parser

**Files:**
- Create: `internal/parser/yaml.go`
- Create: `internal/parser/yaml_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/parser/yaml_test.go`:

```go
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseYAMLFile(t *testing.T) {
	content := `
database:
  password: yaml-secret-password
  host: localhost
api_key: sk-yaml-key-abc123
port: 5432
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, "secrets.yaml")
	os.WriteFile(f, []byte(content), 0644)

	got, err := ParseYAMLFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := map[string]string{
		"database.password": "yaml-secret-password",
		"database.host":     "localhost",
		"api_key":           "sk-yaml-key-abc123",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %s: got %q, want %q", k, got[k], want)
		}
	}
	if _, ok := got["port"]; ok {
		t.Error("numeric value should be excluded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/parser/... -run TestParseYAMLFile -v
```

Expected: compile error.

- [ ] **Step 3: Implement yaml.go**

Create `internal/parser/yaml.go`:

```go
package parser

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ParseYAMLFile parses a YAML file and returns flattened key=value pairs.
// Nested keys are dot-separated. Only string leaf values are included.
func ParseYAMLFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	flattenYAML("", raw, result)
	return result, nil
}

func flattenYAML(prefix string, v interface{}, out map[string]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenYAML(key, child, out)
		}
	case map[interface{}]interface{}:
		for k, child := range val {
			ks, _ := k.(string)
			key := ks
			if prefix != "" {
				key = prefix + "." + ks
			}
			flattenYAML(key, child, out)
		}
	case []interface{}:
		for _, child := range val {
			flattenYAML(prefix, child, out)
		}
	case string:
		if prefix != "" && val != "" {
			out[prefix] = val
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/parser/... -run TestParseYAMLFile -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/yaml.go internal/parser/yaml_test.go
git commit -m "feat: add YAML file parser"
```

---

## Task 5: Credential file discovery

**Files:**
- Create: `internal/parser/discover.go`
- Create: `internal/parser/discover_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/parser/discover_test.go`:

```go
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSecrets(t *testing.T) {
	tmp := t.TempDir()

	// .env file
	os.WriteFile(filepath.Join(tmp, ".env"), []byte("SECRET_KEY=abc12345678\n"), 0644)

	// JSON file
	os.WriteFile(filepath.Join(tmp, "creds.json"), []byte(`{"token":"json-token-xyz9876"}`), 0644)

	// YAML file
	os.WriteFile(filepath.Join(tmp, "secrets.yaml"), []byte("api_key: yaml-key-qwerty123\n"), 0644)

	// properties file
	os.WriteFile(filepath.Join(tmp, "app.properties"), []byte("db.password=props-pass-abc456\n"), 0644)

	// node_modules should be skipped
	os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755)
	os.WriteFile(filepath.Join(tmp, "node_modules", "secret.env"), []byte("SKIP=should-not-load-this\n"), 0644)

	secrets, err := DiscoverSecrets(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if secrets["SECRET_KEY"] != "abc12345678" {
		t.Errorf("missing SECRET_KEY, got: %v", secrets["SECRET_KEY"])
	}
	if secrets["token"] != "json-token-xyz9876" {
		t.Errorf("missing token, got: %v", secrets["token"])
	}
	if secrets["api_key"] != "yaml-key-qwerty123" {
		t.Errorf("missing api_key, got: %v", secrets["api_key"])
	}
	if secrets["db.password"] != "props-pass-abc456" {
		t.Errorf("missing db.password, got: %v", secrets["db.password"])
	}
	if _, ok := secrets["SKIP"]; ok {
		t.Error("node_modules secret should not be loaded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/parser/... -run TestDiscoverSecrets -v
```

Expected: compile error — `DiscoverSecrets` not defined.

- [ ] **Step 3: Implement discover.go**

Create `internal/parser/discover.go`:

```go
package parser

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skipDirs are directories we never descend into.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"target":       true,
	"build":        true,
	"dist":         true,
}

// DiscoverSecrets walks root recursively, parses all credential files it finds,
// and returns a merged map of key -> secret value.
// Values that are too short or look like non-secrets are filtered out.
func DiscoverSecrets(root string) (map[string]string, error) {
	all := make(map[string]string)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		var parsed map[string]string
		var parseErr error

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".json":
			parsed, parseErr = ParseJSONFile(path)
		case ".yaml", ".yml":
			parsed, parseErr = ParseYAMLFile(path)
		default:
			// check if file looks like key=value format
			if looksLikeEnvFile(path) {
				parsed, parseErr = ParseEnvFile(path)
			}
		}

		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "aibodyguard: warning: could not parse %s: %v\n", path, parseErr)
			return nil
		}

		for k, v := range parsed {
			if isLikelySecret(v) {
				all[k] = v
			}
		}
		return nil
	})

	return all, err
}

// looksLikeEnvFile returns true if the file contains at least one KEY=VALUE line.
func looksLikeEnvFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if strings.Contains(line, "=") {
			return true
		}
		if count > 20 { // don't read huge files just to check
			break
		}
	}
	return false
}

// isLikelySecret returns true if a value looks like a real secret.
func isLikelySecret(v string) bool {
	if len(v) < 8 {
		return false
	}
	lower := strings.ToLower(v)
	nonSecrets := []string{
		"true", "false", "null", "none", "undefined",
		"localhost", "127.0.0.1", "0.0.0.0",
	}
	for _, ns := range nonSecrets {
		if lower == ns {
			return false
		}
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return false
	}
	// all digits = likely port/version number
	allDigits := true
	for _, c := range v {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	return !allDigits
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/parser/... -run TestDiscoverSecrets -v
```

Expected: PASS

- [ ] **Step 5: Run all parser tests**

```bash
go test ./internal/parser/... -v
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/parser/discover.go internal/parser/discover_test.go
git commit -m "feat: add recursive credential file discovery"
```

---

## Task 6: Secret scanner

**Files:**
- Create: `internal/scanner/scanner.go`
- Create: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/scanner/scanner_test.go`:

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
	s := New(secrets)

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
	if len(redacted) != 2 {
		t.Errorf("expected 2 redacted keys, got %d: %v", len(redacted), redacted)
	}
}

func TestScanAndRedactNoMatch(t *testing.T) {
	secrets := map[string]string{
		"DB_PASSWORD": "supersecret123",
	}
	s := New(secrets)

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

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/scanner/... -run TestScan -v
```

Expected: compile error — `New` not defined.

- [ ] **Step 3: Implement scanner.go**

Create `internal/scanner/scanner.go`:

```go
package scanner

import "strings"

// Scanner holds loaded secrets and performs redaction.
type Scanner struct {
	secrets map[string]string // key name -> secret value
}

// New creates a Scanner loaded with the given secrets map.
func New(secrets map[string]string) *Scanner {
	return &Scanner{secrets: secrets}
}

// Redact scans body for any known secret values and replaces them with
// [REDACTED:<KEY_NAME>]. Returns the (possibly modified) body and the list
// of key names that were redacted.
func (s *Scanner) Redact(body string) (string, []string) {
	var redactedKeys []string
	result := body

	for key, val := range s.secrets {
		if strings.Contains(result, val) {
			placeholder := "[REDACTED:" + key + "]"
			result = strings.ReplaceAll(result, val, placeholder)
			redactedKeys = append(redactedKeys, key)
		}
	}

	return result, redactedKeys
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/scanner/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat: add secret scanner with redaction"
```

---

## Task 7: HTTP proxy

**Files:**
- Create: `internal/proxy/proxy.go`

- [ ] **Step 1: Implement proxy.go**

Create `internal/proxy/proxy.go`:

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

// realAPIHosts maps known base URL patterns to real API hosts.
var realAPIHosts = map[string]string{
	"anthropic": "https://api.anthropic.com",
	"openai":    "https://api.openai.com",
}

// Proxy is the local HTTP server that intercepts LLM API calls.
type Proxy struct {
	scanner *scanner.Scanner
	server  *http.Server
	port    int
}

// New creates a Proxy with the given scanner.
func New(s *scanner.Scanner) (*Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not bind port: %w", err)
	}

	p := &Proxy{
		scanner: s,
		port:    listener.Addr().(*net.TCPAddr).Port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handle)
	p.server = &http.Server{Handler: mux}

	go p.server.Serve(listener)
	return p, nil
}

// Port returns the port the proxy is listening on.
func (p *Proxy) Port() int {
	return p.port
}

// Shutdown stops the proxy server.
func (p *Proxy) Shutdown() {
	p.server.Close()
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	// Read request body
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}
	r.Body.Close()

	// Redact secrets
	bodyStr := string(bodyBytes)
	cleaned, redacted := p.scanner.Redact(bodyStr)
	for _, key := range redacted {
		fmt.Fprintf(w.(interface{ Header() http.Header }).Header(), "") // flush before log
		fmt.Printf("⚠  Redacted secret: %s\n", key)
	}

	// Determine target API
	targetBase := resolveTarget(r)

	// Build upstream request
	upstreamURL := targetBase + r.URL.RequestURI()
	upstreamReq, err := http.NewRequest(r.Method, upstreamURL, bytes.NewBufferString(cleaned))
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, vals := range r.Header {
		for _, v := range vals {
			upstreamReq.Header.Add(key, v)
		}
	}
	upstreamReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(cleaned)))

	// Forward to real API
	client := &http.Client{}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	io.Copy(w, resp.Body)
}

// resolveTarget determines the real API base URL from the incoming request.
// Falls back to Anthropic if unknown.
func resolveTarget(r *http.Request) string {
	host := strings.ToLower(r.Host)
	path := strings.ToLower(r.URL.Path)
	combined := host + path

	if strings.Contains(combined, "openai") || strings.Contains(path, "/v1/chat") {
		return realAPIHosts["openai"]
	}
	return realAPIHosts["anthropic"]
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
cd ~/Documents/AIBodyguard
go build ./internal/proxy/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/proxy/proxy.go
git commit -m "feat: add local HTTP proxy with request interception"
```

---

## Task 8: CLI entrypoint

**Files:**
- Create: `cmd/aibodyguard/main.go`

- [ ] **Step 1: Implement main.go**

Create `cmd/aibodyguard/main.go`:

```go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/yourusername/aibodyguard/internal/parser"
	"github.com/yourusername/aibodyguard/internal/proxy"
	"github.com/yourusername/aibodyguard/internal/scanner"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		os.Exit(0)
	}

	// Find the -- separator
	sepIdx := -1
	for i, a := range args {
		if a == "--" {
			sepIdx = i
			break
		}
	}

	var agentArgs []string
	if sepIdx >= 0 {
		agentArgs = args[sepIdx+1:]
	} else {
		agentArgs = args
	}

	if len(agentArgs) == 0 {
		fmt.Fprintln(os.Stderr, "aibodyguard: error: no agent command specified")
		printUsage()
		os.Exit(1)
	}

	// Discover secrets in current directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "aibodyguard: scanning for credential files...")
	secrets, err := parser.DiscoverSecrets(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: warning: partial scan error: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "aibodyguard: loaded %d secret values from credential files\n", len(secrets))

	// Start proxy
	s := scanner.New(secrets)
	p, err := proxy.New(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting proxy: %v\n", err)
		os.Exit(1)
	}
	defer p.Shutdown()

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", p.Port())
	fmt.Fprintf(os.Stderr, "aibodyguard: proxy listening on %s\n", proxyURL)

	// Spawn the agent with injected env vars
	cmd := exec.Command(agentArgs[0], agentArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_BASE_URL="+proxyURL,
		"OPENAI_BASE_URL="+proxyURL,
		"OPENAI_API_BASE="+proxyURL,
	)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting agent: %v\n", err)
		os.Exit(1)
	}

	// Forward signals to child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		if cmd.Process != nil {
			cmd.Process.Signal(sig)
		}
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `AIBodyguard — Credential leak prevention for AI coding agents

Usage:
  aibodyguard -- <agent> [agent-args...]
  aibodyguard <agent> [agent-args...]

Examples:
  aibodyguard -- opencode
  aibodyguard -- claude
  aibodyguard -- aider --model claude-3-5-sonnet

AIBodyguard scans the current directory for credential files (.env, JSON, YAML,
.properties), then wraps the specified agent with a local proxy that redacts any
discovered secret values before they reach the LLM API.`)
}
```

- [ ] **Step 2: Build the binary**

```bash
cd ~/Documents/AIBodyguard
go build -o aibodyguard ./cmd/aibodyguard/
```

Expected: `aibodyguard` binary created in current directory.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/aibodyguard/main.go
git commit -m "feat: add CLI entrypoint with process lifecycle management"
```

---

## Task 9: Fix logging in proxy (stderr not response header)

The proxy currently has a bug — logging to `fmt.Printf` inside the handler is fine but the code incorrectly uses `w.(interface{...})`. Fix it.

**Files:**
- Modify: `internal/proxy/proxy.go`

- [ ] **Step 1: Fix the redaction logging in handle()**

Replace the logging block in `handle()`:

```go
// Redact secrets
bodyStr := string(bodyBytes)
cleaned, redacted := p.scanner.Redact(bodyStr)
for _, key := range redacted {
    fmt.Fprintf(os.Stderr, "⚠  Redacted secret: %s\n", key)
}
```

Also add `"os"` to imports in proxy.go.

- [ ] **Step 2: Build to verify**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/proxy/proxy.go
git commit -m "fix: log redaction warnings to stderr correctly"
```

---

## Task 10: Write README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README.md**

Create `README.md`:

```markdown
# AIBodyguard

A credential leak prevention wrapper for AI coding agents.

AIBodyguard sits between your coding agent (OpenCode, Claude Code, Cursor, etc.) and the LLM API. It automatically discovers credential files in your project, then intercepts and redacts any secret values before they reach the AI — without any manual configuration.

## How It Works

1. At startup, AIBodyguard scans your current directory recursively for credential files (`.env`, JSON, YAML, `.properties`)
2. It starts a local HTTP proxy and injects `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL` into the agent's environment
3. All LLM API requests from the agent route through the proxy
4. Secret values are replaced with `[REDACTED:<KEY_NAME>]` before forwarding
5. You see a warning on stderr whenever a redaction occurs

## Installation

### Download binary (macOS/Linux)

```bash
# macOS (Apple Silicon)
curl -L https://github.com/yourusername/aibodyguard/releases/latest/download/aibodyguard-darwin-arm64 -o aibodyguard
chmod +x aibodyguard
sudo mv aibodyguard /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/yourusername/aibodyguard/releases/latest/download/aibodyguard-darwin-amd64 -o aibodyguard
chmod +x aibodyguard
sudo mv aibodyguard /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/yourusername/aibodyguard/releases/latest/download/aibodyguard-linux-amd64 -o aibodyguard
chmod +x aibodyguard
sudo mv aibodyguard /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/yourusername/aibodyguard.git
cd aibodyguard
go build -o aibodyguard ./cmd/aibodyguard/
sudo mv aibodyguard /usr/local/bin/
```

Requires Go 1.22+.

## Usage

```bash
# Wrap OpenCode
aibodyguard -- opencode

# Wrap Claude Code
aibodyguard -- claude

# Wrap any agent
aibodyguard -- <agent-command> [agent-args...]
```

Run from your project root. AIBodyguard will scan the current directory for credentials.

## Supported Credential File Formats

| Format | Examples |
|--------|---------|
| `.env` | `.env`, `.env.local`, `.env.production` |
| `.properties` | `application.properties`, `config.properties` |
| JSON | `credentials.json`, `service-account.json`, `*.json` |
| YAML | `secrets.yaml`, `config.yml`, `*.yaml`, `*.yml` |

## What Gets Redacted

A value is treated as a secret if it:
- Is 8+ characters long
- Is not a common config value (`true`, `false`, `localhost`, URLs, etc.)
- Is not all digits

## Example Output

```
aibodyguard: scanning for credential files...
aibodyguard: loaded 12 secret values from credential files
aibodyguard: proxy listening on http://127.0.0.1:54231
⚠  Redacted secret: DB_PASSWORD
⚠  Redacted secret: OPENAI_API_KEY
```

## Supported Agents

Any agent that respects `ANTHROPIC_BASE_URL` or `OPENAI_BASE_URL` environment variables:

- [OpenCode](https://github.com/opencodelabs/opencode)
- [Claude Code](https://claude.ai/code)
- [Aider](https://aider.chat)
- [Continue](https://continue.dev)
- [Cursor](https://cursor.sh) (via settings)

## License

MIT
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add README with installation and usage instructions"
```

---

## Task 11: Add GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create release workflow**

```bash
mkdir -p .github/workflows
```

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: darwin
            goarch: arm64
          - goos: darwin
            goarch: amd64
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: windows
            goarch: amd64

    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          EXT=""
          if [ "$GOOS" = "windows" ]; then EXT=".exe"; fi
          go build -ldflags="-s -w" -o aibodyguard-${{ matrix.goos }}-${{ matrix.goarch }}${EXT} ./cmd/aibodyguard/

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: aibodyguard-${{ matrix.goos }}-${{ matrix.goarch }}
          path: aibodyguard-*

  release:
    needs: build
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/download-artifact@v4
        with:
          merge-multiple: true

      - name: Create release
        uses: softprops/action-gh-release@v2
        with:
          files: aibodyguard-*
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add GitHub Actions release workflow for cross-platform binaries"
```

---

## Task 12: Add LICENSE

**Files:**
- Create: `LICENSE`

- [ ] **Step 1: Create MIT license**

Create `LICENSE`:

```
MIT License

Copyright (c) 2026 AIBodyguard Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Commit**

```bash
git add LICENSE
git commit -m "chore: add MIT license"
```

---

## Final Verification

- [ ] Run all tests: `go test ./... -v` — all PASS
- [ ] Build binary: `go build -o aibodyguard ./cmd/aibodyguard/` — no errors
- [ ] Smoke test: create a `.env` with a test secret, run `aibodyguard -- echo hello`, confirm proxy starts and secret count is reported
