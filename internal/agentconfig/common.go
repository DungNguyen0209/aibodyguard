package agentconfig

// CommonEnv returns the env vars required by every tool AIBodyguard wraps.
// proxyAddr is the full proxy URL e.g. "http://127.0.0.1:8080".
// caPath is the path to the AIBodyguard CA certificate PEM file.
func CommonEnv(proxyAddr, caPath string) []string {
	return []string{
		"HTTPS_PROXY=" + proxyAddr,
		"https_proxy=" + proxyAddr,
		"NODE_EXTRA_CA_CERTS=" + caPath,
		"SSL_CERT_FILE=" + caPath,
		"REQUESTS_CA_BUNDLE=" + caPath, // Python tools (aider, etc.)
	}
}
