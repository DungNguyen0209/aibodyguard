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
	ca              *ca
	scanner         Redactor
	log             io.Writer
	listener        net.Listener
	port            int
	once            sync.Once
	upstreamTLSConf *tls.Config // nil = use system roots
}

func newMITMProxy(s Redactor, log io.Writer) (MITM, error) {
	return newMITMProxyWithUpstreamTLS(s, log, nil)
}

func newMITMProxyWithUpstreamTLS(s Redactor, log io.Writer, upstreamTLS *tls.Config) (MITM, error) {
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
		listener:        ln,
		port:            ln.Addr().(*net.TCPAddr).Port,
		upstreamTLSConf: upstreamTLS,
	}

	go p.serve()
	return p, nil
}

func (p *mitmProxy) Port() int         { return p.port }
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
		NextProtos:   []string{"http/1.1"},
	})
	if err := tlsClientConn.Handshake(); err != nil {
		fmt.Fprintf(p.log, "[aibodyguard] TLS handshake with client for %s: %v\n", hostname, err)
		return
	}
	defer tlsClientConn.Close()

	// Connect to real upstream over TLS.
	// Force HTTP/1.1 via NextProtos — http.ReadResponse cannot parse HTTP/2 frames.
	upstreamTLSConf := p.upstreamTLSConf
	if upstreamTLSConf == nil {
		upstreamTLSConf = &tls.Config{
			ServerName: hostname,
			NextProtos: []string{"http/1.1"},
		}
	}
	upstreamConn, err := tls.Dial("tcp", host, upstreamTLSConf)
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
	upstreamReader := bufio.NewReader(upstreamConn) // reuse across requests

	for {
		req, err := http.ReadRequest(clientReader)
		if err != nil {
			return
		}

		// Fix Host header — http.ReadRequest may leave it empty for HTTP/1.0
		if req.Host == "" {
			req.Host = hostname
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

		// Forward request to upstream.
		// req.URL from http.ReadRequest has no scheme/host (it's a tunnel request),
		// so req.Write produces a correct relative-path HTTP/1.1 request line.
		req.Body = io.NopCloser(bytes.NewBufferString(cleaned))
		req.ContentLength = int64(len(cleaned))
		// Remove proxy-specific headers that shouldn't reach the upstream
		req.Header.Del("Proxy-Connection")
		if err := req.Write(upstreamConn); err != nil {
			fmt.Fprintf(p.log, "[aibodyguard] write upstream: %v\n", err)
			return
		}

		// Read and stream response back to client
		resp, err := http.ReadResponse(upstreamReader, req)
		if err != nil {
			fmt.Fprintf(p.log, "[aibodyguard] read upstream response: %v\n", err)
			return
		}

		fmt.Fprintf(p.log, "[aibodyguard] ← %d %s\n", resp.StatusCode, resp.Status)

		// Use resp.Write — it correctly handles Transfer-Encoding: chunked,
		// Content-Length, and streaming. It calls io.Copy internally so it
		// does not buffer the full body before writing.
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
