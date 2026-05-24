package mitm

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/yourusername/aibodyguard/internal/logger"
)

func TestProxyLogsRequest(t *testing.T) {
	// Upstream server
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	logFile := t.TempDir() + "/req.log"
	reqLogger, err := logger.New(logFile)
	if err != nil {
		t.Fatal(err)
	}

	sc := &mockRedactor{redactWord: "supersecret123"}
	logBuf := &bytes.Buffer{}
	p, err := newMITMProxyWithUpstreamTLSAndLogger(sc, logBuf, &tls.Config{InsecureSkipVerify: true}, reqLogger) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	defer p.Shutdown()

	// Build a client that trusts our proxy CA
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(p.CACertPEM())
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

	body := `{"key":"supersecret123"}`
	resp, err := client.Post(upstream.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

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
	if !strings.Contains(entry.BodyRedacted, "[REDACTED:SECRET]") {
		t.Errorf("body_redacted should contain redaction marker, got: %q", entry.BodyRedacted)
	}
	if len(entry.RedactedKeys) == 0 || entry.RedactedKeys[0] != "SECRET" {
		t.Errorf("redacted_keys: got %v", entry.RedactedKeys)
	}
}
