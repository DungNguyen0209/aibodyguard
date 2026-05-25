# Per-Tool Agent Config Design

**Date:** 2026-05-25
**Status:** Approved

## Goal

`aibodyguard claude` and `aibodyguard opencode` each inject the correct set of
env vars into the child process for that specific tool, while sharing a common
base config. Each tool's config lives in its own Go file for easy maintenance.

## Architecture

### New package: `internal/agentconfig`

Three files, one responsibility each:

| File | Responsibility |
|---|---|
| `common.go` | `CommonEnv(proxyAddr, caPath string) []string` — vars every tool needs |
| `claude.go` | `ClaudeEnv(proxyAddr, caPath string) []string` — Claude Code-specific additions |
| `opencode.go` | `OpenCodeEnv(proxyAddr, caPath string) []string` — OpenCode-specific additions |

Each `*Env` function returns a slice of `"KEY=VALUE"` strings ready to append
to `os.Environ()`. Tool-specific functions call `CommonEnv` internally and
append their own vars on top — callers never need to call both.

### Tool detection in `main.go`

The first non-flag argument (after optional `--`) is the agent binary name.
`main.go` extracts `filepath.Base(agentArgs[0])` and switches on it:

```
"claude"   → agentconfig.ClaudeEnv(proxyAddr, caPath)
"opencode" → agentconfig.OpenCodeEnv(proxyAddr, caPath)
default    → agentconfig.CommonEnv(proxyAddr, caPath)
```

No other changes to `main.go` — the env slice is passed to `cmd.Env` exactly
as today.

## Env vars per tool

### Common (all tools)
| Var | Value | Reason |
|---|---|---|
| `HTTPS_PROXY` | `http://127.0.0.1:<port>` | Route HTTPS through MITM proxy |
| `https_proxy` | same | Lowercase alias, respected by some libs |
| `NODE_EXTRA_CA_CERTS` | `<caPath>` | Trust AIBodyguard's self-signed CA (Node/Bun) |
| `SSL_CERT_FILE` | `<caPath>` | Trust CA for Go/OpenSSL-based tools |
| `REQUESTS_CA_BUNDLE` | `<caPath>` | Trust CA for Python tools (aider etc.) |

### Claude Code extras
| Var | Value | Reason |
|---|---|---|
| `CLAUDE_CODE_CERT_STORE` | `system` | Use OS trust store so our injected CA (via NODE_EXTRA_CA_CERTS) is honoured |
| `NODE_TLS_REJECT_UNAUTHORIZED` | `1` | Ensure TLS verification stays on |

### OpenCode extras
| Var | Value | Reason |
|---|---|---|
| `NO_PROXY` | `localhost,127.0.0.1` | OpenCode TUI talks to a local HTTP server; must not route that through proxy |
| `no_proxy` | same | Lowercase alias |
| `NODE_TLS_REJECT_UNAUTHORIZED` | `1` | Ensure TLS verification stays on |

## Command syntax change

Old: `aibodyguard -- claude` or `aibodyguard -- opencode`
New: `aibodyguard claude` or `aibodyguard opencode` (the `--` separator is
still accepted for backward compatibility and for passing flags to the tool)

## File structure

```
internal/agentconfig/
    common.go
    common_test.go
    claude.go
    claude_test.go
    opencode.go
    opencode_test.go
```

## Testing

Each file has a unit test that:
1. Calls the `*Env` function with fixed `proxyAddr` and `caPath`
2. Asserts required keys are present with correct values
3. For tool-specific functions, asserts common vars are also present

## Non-goals

- No daemon/background proxy
- No persistent config files written to disk
- No changes to the proxy or scanner internals
