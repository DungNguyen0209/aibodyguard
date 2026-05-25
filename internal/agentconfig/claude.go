package agentconfig

// ClaudeEnv returns the full env var slice for Claude Code (claude CLI).
// It includes all common vars plus Claude Code-specific settings.
//
// Claude Code docs: https://docs.anthropic.com/en/docs/claude-code/network-config
//   - CLAUDE_CODE_CERT_STORE=system  — use OS trust store so NODE_EXTRA_CA_CERTS is honoured
//   - NODE_TLS_REJECT_UNAUTHORIZED=1 — keep TLS verification enabled
func ClaudeEnv(proxyAddr, caPath string) []string {
	env := CommonEnv(proxyAddr, caPath)
	env = append(env,
		"CLAUDE_CODE_CERT_STORE=system",
		"NODE_TLS_REJECT_UNAUTHORIZED=1",
	)
	return env
}
