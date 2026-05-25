package agentconfig

// OpenCodeEnv returns the full env var slice for OpenCode (opencode CLI).
// It includes all common vars plus OpenCode-specific settings.
//
// OpenCode docs: https://opencode.ai/docs/network/
//   - NO_PROXY=localhost,127.0.0.1  — OpenCode TUI talks to a local HTTP server;
//     routing that through the proxy causes a loop and breaks the TUI.
//   - NODE_TLS_REJECT_UNAUTHORIZED=1 — keep TLS verification enabled
func OpenCodeEnv(proxyAddr, caPath string) []string {
	env := CommonEnv(proxyAddr, caPath)
	env = append(env,
		"NO_PROXY=localhost,127.0.0.1",
		"no_proxy=localhost,127.0.0.1",
		"NODE_TLS_REJECT_UNAUTHORIZED=1",
	)
	return env
}
