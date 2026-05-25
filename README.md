# AIBodyguard

A credential leak prevention wrapper for AI coding agents.

AIBodyguard sits between your coding agent (OpenCode, Claude Code, etc.) and the LLM API. It automatically discovers credential files in your project, intercepts all outbound HTTPS requests, and redacts any secret values before they reach the AI — without any manual configuration.

## How It Works

1. At startup, AIBodyguard scans your current directory recursively for credential files (`.env`, JSON, YAML, `.properties`)
2. It generates an ephemeral CA certificate and starts a local TLS MITM proxy
3. The agent is launched with `HTTPS_PROXY` and `NODE_EXTRA_CA_CERTS` injected — no changes to the agent's own config needed
4. Every outbound HTTPS request is intercepted; secret values in the body are replaced with `****` before forwarding to the API
5. In `--test` mode, full request details are also written to a JSON log file for inspection

## Installation

### Build from source

```bash
git clone https://github.com/DungNguyen0209/aibodyguard.git
cd aibodyguard
go build -o aibodyguard ./cmd/aibodyguard/
sudo mv aibodyguard /usr/local/bin/
```

Requires Go 1.22+.

## Usage

```bash
# Wrap Claude Code
aibodyguard claude

# Wrap OpenCode
aibodyguard opencode

# Wrap any other agent
aibodyguard <agent-command> [agent-args...]

# Pass flags to the agent using -- separator
aibodyguard -- claude --some-flag
```

Run from your project root. AIBodyguard scans the current directory for credentials on every run.

### --test mode

By default AIBodyguard only redacts — it does not write any request data to disk. Use `--test` to enable full request logging for inspection and debugging:

```bash
aibodyguard --test claude
aibodyguard --test opencode
```

When `--test` is active:

- Every intercepted request is appended as a JSON line to `/tmp/aibodyguard-requests.log`
- The startup banner shows `Mode : TEST (request log active)`
- The log file contains:

| Field | Description |
|---|---|
| `timestamp` | UTC time the request was intercepted |
| `method` | HTTP method (POST, GET, …) |
| `url` | Full `https://` URL |
| `headers` | All request headers (including `Authorization`) |
| `body_original` | Raw request body before redaction |
| `body_redacted` | Request body with secrets replaced by `****` |
| `redacted_keys` | List of secret values that were matched and replaced |

> **Note:** `body_original` contains real secret values. Keep the log file private and delete it when done.

To inspect the log:

```bash
# Pretty-print the latest request
tail -1 /tmp/aibodyguard-requests.log | jq .

# Show only redacted requests
jq 'select(.redacted_keys | length > 0)' /tmp/aibodyguard-requests.log

# Watch live
tail -f /tmp/aibodyguard-requests.log | jq .
```

## Supported Credential File Formats

AIBodyguard parses files whose name matches known credential patterns:

| Format | Matched when filename… | Examples |
|---|---|---|
| `.env` | is `.env`, `.env.*`, `.envrc` | `.env`, `.env.local`, `.env.production` |
| `.properties` | any `.properties` file | `application.properties` |
| JSON | contains `config`, `secret`, `credential`, `setting`, `value` | `appsettings.json`, `credentials.json` |
| YAML | contains `config`, `secret`, `value`, `setting`, or known names | `secrets.yaml`, `values.yaml`, `appsettings-prod.yml` |

Directories skipped: `node_modules`, `.git`, `vendor`, `build`, `dist`, and localization trees (`i18n`, `locales`, `translations`, …).

## What Gets Redacted

A value is treated as a secret if it:

- Is 10+ characters long
- Is not a common non-secret (`true`, `false`, `localhost`, plain URLs, cron expressions, etc.)
- Has sufficient complexity: mixed case + digits, or special characters, or length ≥ 32

`jdbc:` connection strings are treated as secrets regardless of the above rules.

## Per-Tool Proxy Configuration

AIBodyguard injects different env vars depending on which tool is being wrapped:

| Env var | Claude Code | OpenCode | Other tools |
|---|---|---|---|
| `HTTPS_PROXY` / `https_proxy` | yes | yes | yes |
| `NODE_EXTRA_CA_CERTS` | yes | yes | yes |
| `SSL_CERT_FILE` | yes | yes | yes |
| `REQUESTS_CA_BUNDLE` | yes | yes | yes |
| `CLAUDE_CODE_CERT_STORE=system` | yes | — | — |
| `NODE_TLS_REJECT_UNAUTHORIZED=1` | yes | yes | — |
| `NO_PROXY=localhost,127.0.0.1` | — | yes | — |

OpenCode requires `NO_PROXY` because its TUI communicates with a local HTTP server — routing that through the proxy would cause a connection loop.

## Startup Banner

```
  AIBodyguard  active
  ─────────────────────────────────────────
  Tool           : claude
  Secrets loaded : 315 values
  Mode           : TEST (request log active)
  Request log    : /tmp/aibodyguard-requests.log
  MITM proxy     : http://127.0.0.1:58368
  CA cert        : /tmp/aibodyguard-ca.pem
  Log            : /tmp/aibodyguard.log
  ─────────────────────────────────────────
```

## Diagnostic Log

All proxy activity (secrets discovered at startup, redaction events, errors) is written to `/tmp/aibodyguard.log`. This file is separate from the request log and is always written regardless of `--test` mode.

## License

MIT
