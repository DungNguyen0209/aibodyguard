# TLS MITM Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current path-prefix HTTP proxy and opencode.json patching with a TLS MITM proxy that intercepts all outbound HTTPS traffic from the wrapped agent, guaranteeing no secrets leak regardless of which API endpoint is called.

**Architecture:** AIBodyguard generates an in-memory CA cert + key at startup, writes the CA cert to a temp file, then starts a TLS-terminating CONNECT proxy on a random local port. The agent process is spawned with `HTTPS_PROXY=http://127.0.0.1:<port>` and `NODE_EXTRA_CA_CERTS=<ca-cert-path>` injected as env vars — so only the child process trusts our CA. For each CONNECT tunnel the proxy: accepts the TLS handshake using a dynamically-signed leaf cert, reads the request body, runs redaction, then forwards to the real upstream over a new TLS connection. The `opencode.json` patching and path-prefix routing are removed entirely.

**Tech Stack:** Go 1.22+ standard library (`crypto/tls`, `crypto/x509`, `crypto/rsa`, `net/http`), no new dependencies.

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `internal/mitm/ca.go` | Create | Generate ephemeral CA cert+key; sign leaf certs per hostname |
| `internal/mitm/proxy.go` | Create | CONNECT proxy: accept tunnel, TLS handshake, read/redact/forward |
| `internal/mitm/mitm.go` | Create | `MITM` interface + `New()` constructor |
| `internal/mitm/ca_test.go` | Create | Test CA cert generation and leaf cert signing |
| `internal/mitm/proxy_test.go` | Create | Test CONNECT tunnel and redaction end-to-end |
| `cmd/aibodyguard/main.go` | Modify | Replace proxy+patchOpencodeConfig with mitm; inject env vars |
| `internal/proxy/proxy.go` | Delete (keep file, gut content) | Old HTTP proxy is no longer used — keep package but empty |

---

## Task 1: CA certificate generation (`internal/mitm/ca.go`)

**Files:**
- Create: `internal/mitm/ca_test.go`
- Create: `internal/mitm/ca.go`

- [ ] **Step 1: Create the failing test**

Create `internal/mitm/ca_test.go`:

```go
package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	if ca.cert == nil {
		t.Fatal("expected non-nil cert")
	}
	if ca.key == nil {
		t.Fatal("expected non-nil key")
	}
	if ca.cert.IsCA != true {
		t.Error("cert should be a CA")
	}
}

func TestSignLeaf(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}

	tlsCert, err := ca.signLeaf("api.githubcopilot.com")
	if err != nil {
		t.Fatalf("signLeaf: %v", err)
	}

	// Parse and verify the leaf cert is signed by our CA
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	_, err = leaf.Verify(x509.VerifyOptions{
		DNSName:     "api.githubcopilot.com",
		Roots:       pool,
		CurrentTime: time.Now(),
	})
	if err != nil {
		t.Errorf("leaf cert not valid for host: %v", err)
	}

	// Verify TLS config works
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	if len(tlsCfg.Certificates) != 1 {
		t.Error("expected 1 certificate in TLS config")
	}
}

func TestCAPEMBytes(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	pem := ca.pemBytes()
	if len(pem) == 0 {
		t.Error("expected non-empty PEM bytes")
	}
	// Should be parseable
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Error("PEM bytes could not be parsed into cert pool")
	}
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go test ./internal/mitm/...
```

Expected: `cannot find package` or `no Go files`

- [ ] **Step 3: Create `internal/mitm/ca.go`**

```go
package mitm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// ca holds an ephemeral certificate authority used to sign leaf certs on demand.
type ca struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	tlsCert tls.Certificate
}

// generateCA creates a new ephemeral CA cert and key.
func generateCA() (*ca, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"AIBodyguard Ephemeral CA"},
			CommonName:   "AIBodyguard CA",
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return &ca{cert: cert, key: key, tlsCert: tlsCert}, nil
}

// signLeaf creates a TLS leaf certificate for the given hostname, signed by this CA.
func (c *ca) signLeaf(hostname string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		DNSNames:     []string{hostname},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("sign leaf cert: %w", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}

// pemBytes returns the CA certificate as PEM-encoded bytes suitable for
// writing to NODE_EXTRA_CA_CERTS or SSL_CERT_FILE.
func (c *ca) pemBytes() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: c.cert.Raw,
	})
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go test ./internal/mitm/...
```

Expected: `ok  github.com/yourusername/aibodyguard/internal/mitm`

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/AIBodyguard
git add internal/mitm/ca.go internal/mitm/ca_test.go
git commit -m "feat(mitm): add ephemeral CA cert generation and leaf signing"
```

---

## Task 2: MITM CONNECT proxy (`internal/mitm/proxy.go` + `mitm.go`)

**Files:**
- Create: `internal/mitm/mitm.go`
- Create: `internal/mitm/proxy.go`
- Create: `internal/mitm/proxy_test.go`

- [ ] **Step 1: Create `internal/mitm/mitm.go` — interface**

```go
package mitm

import "io"

// MITM is a TLS-intercepting CONNECT proxy.
type MITM interface {
	// Port returns the local port the proxy listens on.
	Port() int
	// CACertPEM returns the ephemeral CA certificate in PEM format.
	// Inject this into the child process via NODE_EXTRA_CA_CERTS / SSL_CERT_FILE.
	CACertPEM() []byte
	// Shutdown stops the proxy.
	Shutdown()
}

// New creates and starts a MITM proxy. s is used to redact secrets from
// request bodies. log receives diagnostic lines.
func New(s Redactor, log io.Writer) (MITM, error) {
	return newMITMProxy(s, log)
}

// Redactor redacts known secret values from text.
// scanner.Scanner satisfies this interface.
type Redactor interface {
	Redact(input string) (cleaned string, redactedKeys []string)
}
```

- [ ] **Step 2: Create `internal/mitm/proxy.go` — CONNECT tunnel implementation**

```go
package mitm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// mitmProxy is the concrete MITM implementation.
type mitmProxy struct {
	ca       *ca
	scanner  Redactor
	log      io.Writer
	listener net.Listener
	port     int
	once     sync.Once
}

func newMITMProxy(s Redactor, log io.Writer) (MITM, error) {
	authority, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	p := &mitmProxy{
		ca:       authority,
		scanner:  s,
		log:      log,
		listener: ln,
		port:     ln.Addr().(*net.TCPAddr).Port,
	}

	go p.serve()
	return p, nil
}

func (p *mitmProxy) Port() int        { return p.port }
func (p *mitmProxy) CACertPEM() []byte { return p.ca.pemBytes() }
func (p *mitmProxy) Shutdown()         { p.once.Do(func() { p.listener.Close() }) }

func (p *mitmProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go p.handleConn(conn)
	}
}

// handleConn handles one client connection.
// It expects an HTTP CONNECT request, performs the TLS handshake, then
// proxies requests with redaction applied to the body.
func (p *mitmProxy) handleConn(clientConn net.Conn) {
	defer clientConn.Close()

	// Read the CONNECT request
	br := bufio.NewReader(clientConn)
	req, err := http.ReadRequest(br)
	if err != nil {
		fmt.Fprintf(p.log, "[aibodyguard] read CONNECT: %v\n", err)
		return
	}

	if req.Method != http.MethodConnect {
		fmt.Fprintf(p.log, "[aibodyguard] unexpected method: %s\n", req.Method)
		return
	}

	// Extract host and port
	host := req.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}
	hostname := strings.Split(host, ":")[0]

	// Acknowledge the CONNECT tunnel
	_, err = fmt.Fprint(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	if err != nil {
		return
	}

	// TLS handshake with client using a leaf cert signed by our CA
	leafCert, err := p.ca.signLeaf(hostname)
	if err != nil {
		fmt.Fprintf(p.log, "[aibodyguard] sign leaf for %s: %v\n", hostname, err)
		return
	}

	tlsClientConn := tls.Server(clientConn, &tls.Config{
		Certificates: []tls.Certificate{leafCert},
	})
	if err := tlsClientConn.Handshake(); err != nil {
		fmt.Fprintf(p.log, "[aibodyguard] TLS handshake with client for %s: %v\n", hostname, err)
		return
	}
	defer tlsClientConn.Close()

	// Connect to real upstream over TLS
	upstreamConn, err := tls.Dial("tcp", host, &tls.Config{
		ServerName: hostname,
	})
	if err != nil {
		fmt.Fprintf(p.log, "[aibodyguard] dial upstream %s: %v\n", host, err)
		return
	}
	defer upstreamConn.Close()

	// Now proxy HTTP requests over the established tunnels, redacting bodies
	p.proxyHTTP(tlsClientConn, upstreamConn, hostname)
}

// proxyHTTP reads HTTP requests from clientConn, redacts bodies, forwards to
// upstreamConn, and streams responses back. Handles multiple requests (keep-alive).
func (p *mitmProxy) proxyHTTP(clientConn, upstreamConn net.Conn, hostname string) {
	clientReader := bufio.NewReader(clientConn)

	for {
		req, err := http.ReadRequest(clientReader)
		if err != nil {
			return
		}

		// Read and redact body
		bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024))
		req.Body.Close()
		if err != nil {
			fmt.Fprintf(p.log, "[aibodyguard] read body: %v\n", err)
			return
		}

		cleaned, redacted := p.scanner.Redact(string(bodyBytes))
		for _, key := range redacted {
			fmt.Fprintf(p.log, "[aibodyguard] redacted: %s\n", key)
		}

		fmt.Fprintf(p.log, "[aibodyguard] → %s https://%s%s\n", req.Method, hostname, req.URL.RequestURI())

		// Forward request to upstream
		req.Body = io.NopCloser(bytes.NewBufferString(cleaned))
		req.ContentLength = int64(len(cleaned))
		if err := req.Write(upstreamConn); err != nil {
			fmt.Fprintf(p.log, "[aibodyguard] write upstream: %v\n", err)
			return
		}

		// Read and stream response back to client
		upstreamReader := bufio.NewReader(upstreamConn)
		resp, err := http.ReadResponse(upstreamReader, req)
		if err != nil {
			fmt.Fprintf(p.log, "[aibodyguard] read upstream response: %v\n", err)
			return
		}

		fmt.Fprintf(p.log, "[aibodyguard] ← %d %s\n", resp.StatusCode, resp.Status)

		if err := resp.Write(clientConn); err != nil {
			resp.Body.Close()
			return
		}
		resp.Body.Close()

		// If connection is not keep-alive, stop
		if resp.Close || req.Close {
			return
		}
	}
}
```

- [ ] **Step 3: Create `internal/mitm/proxy_test.go`**

```go
package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockRedactor is a test implementation of Redactor.
type mockRedactor struct {
	redactWord string
}

func (m *mockRedactor) Redact(input string) (string, []string) {
	if m.redactWord != "" && strings.Contains(input, m.redactWord) {
		return strings.ReplaceAll(input, m.redactWord, "[REDACTED:SECRET]"), []string{"SECRET"}
	}
	return input, nil
}

func TestMITMProxyInterface(t *testing.T) {
	p, err := New(&mockRedactor{}, io.Discard)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Shutdown()

	if p.Port() == 0 {
		t.Error("expected non-zero port")
	}
	if len(p.CACertPEM()) == 0 {
		t.Error("expected non-empty CA PEM")
	}

	// CA PEM must be parseable
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(p.CACertPEM()) {
		t.Error("CACertPEM is not valid PEM")
	}
}

func TestMITMProxyRedactsBody(t *testing.T) {
	// Start a real TLS upstream test server
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Echo the body back so we can inspect what was sent
		w.Write(body)
	}))
	defer upstream.Close()

	redactor := &mockRedactor{redactWord: "mysecret"}
	p, err := newMITMProxy(redactor, io.Discard)
	if err != nil {
		t.Fatalf("newMITMProxy: %v", err)
	}
	defer p.Shutdown()

	// Build a client that trusts our CA and uses our proxy
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(p.CACertPEM())

	// Also trust the upstream test server's cert
	for _, cert := range upstream.TLS.Certificates {
		for _, certBytes := range cert.Certificate {
			c, _ := x509.ParseCertificate(certBytes)
			if c != nil {
				caPool.AddCert(c)
			}
		}
	}

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", p.Port())
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: caPool},
		Proxy:           func(r *http.Request) (*url.URL, error) { return url.Parse(proxyURL) },
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Post(upstream.URL, "application/json", strings.NewReader(`{"key":"mysecret"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if strings.Contains(string(body), "mysecret") {
		t.Errorf("expected secret to be redacted, got: %s", body)
	}
	if !strings.Contains(string(body), "[REDACTED:SECRET]") {
		t.Errorf("expected [REDACTED:SECRET] in body, got: %s", body)
	}
}
```

- [ ] **Step 4: Add missing `net/url` import to proxy_test.go**

The test uses `url.Parse` — add import at the top of `proxy_test.go`:

```go
import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)
```

- [ ] **Step 5: Run tests**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go test ./internal/mitm/...
```

Expected: `ok  github.com/yourusername/aibodyguard/internal/mitm`

- [ ] **Step 6: Commit**

```bash
cd ~/Documents/AIBodyguard
git add internal/mitm/mitm.go internal/mitm/proxy.go internal/mitm/proxy_test.go
git commit -m "feat(mitm): add TLS CONNECT proxy with body redaction"
```

---

## Task 3: Update `main.go` — swap to MITM proxy, remove opencode.json patching

**Files:**
- Modify: `cmd/aibodyguard/main.go`

- [ ] **Step 1: Replace `main.go` with the new version**

Replace the entire file with:

```go
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yourusername/aibodyguard/internal/mitm"
	"github.com/yourusername/aibodyguard/internal/parser"
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

	// Open log file — all mid-session output goes here, never to stderr
	logPath := filepath.Join(os.TempDir(), "aibodyguard.log")
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	logWriter := io.Writer(os.Stderr)
	if logErr == nil {
		logWriter = logFile
		defer logFile.Close()
	}

	// Discover secrets in current directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(logWriter, "[aibodyguard] scanning for credential files in %s...\n", cwd)
	secrets, err := parser.New().Discover(cwd)
	if err != nil {
		fmt.Fprintf(logWriter, "[aibodyguard] warning: partial scan error: %v\n", err)
	}

	// Start TLS MITM proxy
	s := scanner.New(secrets)
	p, err := mitm.New(s, logWriter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting proxy: %v\n", err)
		os.Exit(1)
	}
	defer p.Shutdown()

	// Write CA cert to temp file so child process can trust it
	caPath := filepath.Join(os.TempDir(), "aibodyguard-ca.pem")
	if err := os.WriteFile(caPath, p.CACertPEM(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error writing CA cert: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(caPath)

	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", p.Port())

	// ── Startup banner ──
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  AIBodyguard  active\n")
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Secrets loaded : %d values\n", len(secrets))
	fmt.Fprintf(os.Stderr, "  MITM proxy     : %s\n", proxyAddr)
	fmt.Fprintf(os.Stderr, "  CA cert        : %s\n", caPath)
	fmt.Fprintf(os.Stderr, "  Log            : %s\n", logPath)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "\n")

	// Spawn the agent with proxy + CA cert injected into env only
	cmd := exec.Command(agentArgs[0], agentArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"HTTPS_PROXY="+proxyAddr,
		"https_proxy="+proxyAddr,
		"NODE_EXTRA_CA_CERTS="+caPath,
		"NODE_TLS_REJECT_UNAUTHORIZED=1",
		"SSL_CERT_FILE="+caPath,
		"REQUESTS_CA_BUNDLE="+caPath, // Python (aider, etc.)
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
			cmd.Process.Signal(sig) //nolint:errcheck
		}
	}()

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// ── Exit line ──
	fmt.Fprintf(os.Stderr, "\n  AIBodyguard  session ended at %s  |  log: %s\n\n",
		time.Now().Format("15:04:05"), logPath)

	os.Exit(exitCode)
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
.properties), starts a TLS MITM proxy, and wraps the agent with HTTPS_PROXY +
NODE_EXTRA_CA_CERTS so all outbound HTTPS traffic is intercepted and secrets
are redacted before they reach any LLM API.`)
}
```

- [ ] **Step 2: Build**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go build ./...
```

Expected: success, binary produced.

- [ ] **Step 3: Smoke test (no real agent)**

```bash
cd ~/Documents/AIBodyguard
./aibodyguard -- echo ok
```

Expected: startup banner prints, `ok`, exit line. CA cert file created then cleaned up.

- [ ] **Step 4: Run all tests**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/AIBodyguard
git add cmd/aibodyguard/main.go
git commit -m "feat(main): replace HTTP proxy with TLS MITM; remove opencode.json patching"
```

---

## Task 4: Remove old proxy code + clean up

**Files:**
- Modify: `internal/proxy/proxy.go` — gut the old HTTP proxy (it's no longer used)

- [ ] **Step 1: Replace `internal/proxy/proxy.go` with a tombstone comment**

Replace the entire file with:

```go
// Package proxy is retained for compatibility but is no longer used.
// The active interceptor is internal/mitm.
package proxy
```

- [ ] **Step 2: Build to confirm nothing breaks**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
go build ./...
go test ./...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
cd ~/Documents/AIBodyguard
git add internal/proxy/proxy.go
git commit -m "chore(proxy): retire old HTTP proxy package; superseded by mitm"
```

---

## Task 5: Live smoke test with OpenCode

This is a manual verification step — no code changes.

- [ ] **Step 1: Build the final binary**

```bash
export PATH="/opt/homebrew/bin:$PATH"
cd ~/Documents/AIBodyguard
make build
```

- [ ] **Step 2: Clear the log**

```bash
> /var/folders/sc/wshn1_b11nb991brz7xyhvps5ll62m/T/aibodyguard.log
```

- [ ] **Step 3: Run against mya-go-spr-services**

```bash
cd ~/Documents/mya-go-spr-services
~/Documents/AIBodyguard/aibodyguard -- opencode
```

- [ ] **Step 4: Ask a question in OpenCode**

Type something simple: `what is this project?`

- [ ] **Step 5: Check the log**

```bash
cat /var/folders/sc/wshn1_b11nb991brz7xyhvps5ll62m/T/aibodyguard.log
```

Expected:
- `→ POST https://api.githubcopilot.com/...` lines showing real upstream calls
- `← 200` responses
- Any `redacted:` lines showing secrets caught
- No 404 errors
