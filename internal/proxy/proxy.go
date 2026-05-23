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
