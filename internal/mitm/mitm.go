package mitm

import (
	"fmt"
	"io"

	"github.com/DungNguyen0209/aibodyguard/internal/logger"
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
	// EnableRequestLog controls whether intercepted requests are written to a
	// JSON log file. Disabled by default; enabled with the --test flag.
	EnableRequestLog bool

	// RequestLogPath is the file path for the JSON request log.
	// Only used when EnableRequestLog is true.
	// Defaults to /tmp/aibodyguard-requests.log if empty.
	RequestLogPath string
}

// New creates and starts a MITM proxy. s is used to redact secrets from
// request bodies. log receives diagnostic lines. cfg may be nil to use defaults.
func New(s Redactor, log io.Writer, cfg *Config) (MITM, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	var reqLogger *logger.RequestLogger
	if cfg.EnableRequestLog {
		path := cfg.RequestLogPath
		if path == "" {
			path = "/tmp/aibodyguard-requests.log"
		}
		var err error
		reqLogger, err = logger.New(path)
		if err != nil {
			fmt.Fprintf(log, "[aibodyguard] WARNING: could not open request log %s: %v\n", path, err)
			reqLogger = nil
		} else {
			fmt.Fprintf(log, "[aibodyguard] request log: %s\n", path)
		}
	}

	return newMITMProxyWithUpstreamTLSAndLogger(s, log, nil, reqLogger)
}

// Redactor redacts known secret values from text.
// scanner.Scanner satisfies this interface.
type Redactor interface {
	Redact(input string) (cleaned string, redactedKeys []string)
}
