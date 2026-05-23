package mitm

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
	p, err := newMITMProxyWithUpstreamTLS(redactor, io.Discard, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
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
