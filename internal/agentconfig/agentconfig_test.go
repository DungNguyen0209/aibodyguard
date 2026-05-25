package agentconfig

import (
	"strings"
	"testing"
)

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func TestCommonEnv(t *testing.T) {
	proxy := "http://127.0.0.1:9999"
	ca := "/tmp/ca.pem"
	m := envMap(CommonEnv(proxy, ca))

	cases := map[string]string{
		"HTTPS_PROXY":        proxy,
		"https_proxy":        proxy,
		"NODE_EXTRA_CA_CERTS": ca,
		"SSL_CERT_FILE":      ca,
		"REQUESTS_CA_BUNDLE": ca,
	}
	for k, want := range cases {
		if got, ok := m[k]; !ok {
			t.Errorf("CommonEnv: missing key %q", k)
		} else if got != want {
			t.Errorf("CommonEnv: %s = %q, want %q", k, got, want)
		}
	}
}

func TestClaudeEnv_includesCommon(t *testing.T) {
	proxy := "http://127.0.0.1:9999"
	ca := "/tmp/ca.pem"
	m := envMap(ClaudeEnv(proxy, ca))

	// Must include all common vars
	commonKeys := []string{"HTTPS_PROXY", "https_proxy", "NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "REQUESTS_CA_BUNDLE"}
	for _, k := range commonKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("ClaudeEnv: missing common key %q", k)
		}
	}
}

func TestClaudeEnv_claudeSpecific(t *testing.T) {
	m := envMap(ClaudeEnv("http://127.0.0.1:9999", "/tmp/ca.pem"))

	if m["CLAUDE_CODE_CERT_STORE"] != "system" {
		t.Errorf("ClaudeEnv: CLAUDE_CODE_CERT_STORE = %q, want %q", m["CLAUDE_CODE_CERT_STORE"], "system")
	}
	if m["NODE_TLS_REJECT_UNAUTHORIZED"] != "1" {
		t.Errorf("ClaudeEnv: NODE_TLS_REJECT_UNAUTHORIZED = %q, want %q", m["NODE_TLS_REJECT_UNAUTHORIZED"], "1")
	}
}

func TestOpenCodeEnv_includesCommon(t *testing.T) {
	proxy := "http://127.0.0.1:9999"
	ca := "/tmp/ca.pem"
	m := envMap(OpenCodeEnv(proxy, ca))

	commonKeys := []string{"HTTPS_PROXY", "https_proxy", "NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "REQUESTS_CA_BUNDLE"}
	for _, k := range commonKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("OpenCodeEnv: missing common key %q", k)
		}
	}
}

func TestOpenCodeEnv_openCodeSpecific(t *testing.T) {
	m := envMap(OpenCodeEnv("http://127.0.0.1:9999", "/tmp/ca.pem"))

	for _, k := range []string{"NO_PROXY", "no_proxy"} {
		if !strings.Contains(m[k], "localhost") {
			t.Errorf("OpenCodeEnv: %s = %q, want it to contain 'localhost'", k, m[k])
		}
		if !strings.Contains(m[k], "127.0.0.1") {
			t.Errorf("OpenCodeEnv: %s = %q, want it to contain '127.0.0.1'", k, m[k])
		}
	}
	if m["NODE_TLS_REJECT_UNAUTHORIZED"] != "1" {
		t.Errorf("OpenCodeEnv: NODE_TLS_REJECT_UNAUTHORIZED = %q, want %q", m["NODE_TLS_REJECT_UNAUTHORIZED"], "1")
	}
}
