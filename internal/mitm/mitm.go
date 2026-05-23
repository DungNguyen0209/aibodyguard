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
