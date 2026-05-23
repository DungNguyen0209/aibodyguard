package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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

// New creates a Proxy with the given scanner, starts listening on a random port.
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
	// Read request body (limit to 10MB)
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
		fmt.Fprintf(os.Stderr, "⚠  Redacted secret: %s\n", key)
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

	// Copy headers (preserve auth token and all other headers)
	for key, vals := range r.Header {
		for _, v := range vals {
			upstreamReq.Header.Add(key, v)
		}
	}
	upstreamReq.ContentLength = int64(len(cleaned))

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
