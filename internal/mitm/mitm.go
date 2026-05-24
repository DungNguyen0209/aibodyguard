package mitm

import (
	"fmt"
	"io"

	"github.com/yourusername/aibodyguard/internal/logger"
)

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

// Config holds options for creating a MITM proxy.
type Config struct {
	// RequestLogPath is the file path for the JSON request log.
	// Defaults to /tmp/aibodyguard-requests.log if empty.
	RequestLogPath string
}

// New creates and starts a MITM proxy. s is used to redact secrets from
// request bodies. log receives diagnostic lines. cfg may be nil to use defaults.
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
// scanner.Scanner satisfies this interface.
type Redactor interface {
	Redact(input string) (cleaned string, redactedKeys []string)
}
