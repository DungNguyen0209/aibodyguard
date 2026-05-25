# AIBodyguard — Design Spec

**Date:** 2026-05-22  
**Status:** Approved

---

## Overview

AIBodyguard is a Go CLI tool that wraps AI coding agents (OpenCode, Claude Code, Cursor, etc.) and prevents credential leaks by intercepting outbound LLM API requests, scanning them for secret values, redacting any matches, and forwarding the clean request to the real API.

Users run:
```
aibodyguard -- opencode
aibodyguard -- claude
aibodyguard -- <any-agent>
```

---

## Problem

Coding agents read files from your project and send their contents to LLM APIs. If those files contain credentials (API keys, passwords, tokens), those secrets get sent to external services — a significant security and compliance risk.

---

## Approach: CLI Wrapper + Base URL Override

AIBodyguard spawns the coding agent as a child process after injecting environment variables that redirect LLM API traffic to a local HTTP proxy:

```
ANTHROPIC_BASE_URL=http://localhost:<port>
OPENAI_BASE_URL=http://localhost:<port>
OPENAI_API_BASE=http://localhost:<port>
```

The local proxy intercepts each request, scans the body for known secret values, redacts them, then forwards the clean request to the real API endpoint. No TLS MITM or certificate management required.

---

## Architecture

```
User: aibodyguard -- opencode
          │
          ▼
┌─────────────────────────┐
│   CLI Entrypoint        │  parses args, starts proxy, injects env, spawns agent
│   cmd/aibodyguard/      │
└────────────┬────────────┘
             │ ANTHROPIC_BASE_URL=http://localhost:<port>
             ▼
┌─────────────────────────┐
│   Local HTTP Proxy      │  receives all LLM API calls from agent
│   internal/proxy/       │  forwards to real API after redaction
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│   Secret Scanner        │  scans raw request JSON for secret values
│   internal/scanner/     │  replaces matches with [REDACTED:<KEY_NAME>]
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│   File Parsers          │  load secret key=value pairs at startup
│   internal/parser/      │  from auto-discovered credential files
└─────────────────────────┘
```

---

## Secret Discovery

At startup, AIBodyguard walks the **current working directory recursively** and collects secrets from:

| File Type | Discovery Rule |
|-----------|---------------|
| JSON | All `*.json` files |
| YAML | All `*.yaml` and `*.yml` files |
| .env / .properties | Any file containing at least one line matching `*=*` |

**Excluded directories:** `node_modules/`, `.git/`, `vendor/`, `target/`, `build/`, `dist/`

**Value filtering (avoid false positives):**
- Value length must be >= 8 characters
- Value must not be a known non-secret: `true`, `false`, `null`, `localhost`, values starting with `http://` or `https://`
- Value must not be all digits (likely a port or version number)

---

## Parsing Rules

### .env / .properties
- Split each line on the first `=`
- Left side = key name, right side = value
- Strip quotes (`"`, `'`) from values
- Skip comment lines (`#`, `!`)

### JSON
- Recursively flatten all string leaf values
- Key name = dot-separated JSON path (e.g., `database.password`)

### YAML
- Recursively flatten all string leaf values
- Key name = dot-separated YAML path

---

## Redaction

When a secret value is found in an outbound request body:
1. Replace the value string with `[REDACTED:<KEY_NAME>]`
2. Print to stderr: `⚠  Redacted secret: <KEY_NAME>`
3. Forward the modified request body to the real API

The original auth token (Bearer token from the agent) is preserved and forwarded — AIBodyguard only redacts values it loaded from credential files.

---

## Proxy Mechanics

- Starts on a random available port (`:0`) at launch
- Handles all HTTP methods (POST, GET, etc.)
- Preserves original path, query string, and all headers
- Replaces `Host` header with real API host
- Streams response back to the agent unchanged (supports SSE / chunked streaming)
- Real API base URLs:
  - Anthropic: `https://api.anthropic.com`
  - OpenAI: `https://api.openai.com`
  - Detected from the `Host` header or request path

---

## Project Structure

```
AIBodyguard/
├── cmd/
│   └── aibodyguard/
│       └── main.go           # CLI entrypoint, arg parsing, process lifecycle
├── internal/
│   ├── proxy/
│   │   └── proxy.go          # HTTP server, request interception, forwarding
│   ├── scanner/
│   │   └── scanner.go        # Secret scanning and redaction logic
│   └── parser/
│       ├── env.go             # .env and .properties parser
│       ├── json.go            # JSON file parser
│       └── yaml.go            # YAML file parser
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Port already in use | Pick next available port |
| Credential file unreadable | Log warning, skip file, continue |
| Agent process exits | AIBodyguard exits with same exit code |
| Real API unreachable | Return 502 to agent, log error |
| Request body too large (>10MB) | Forward without scanning, log warning |

---

## Out of Scope (v1)

- TOML file support
- Web UI or dashboard
- Audit log to file
- Response scanning (only requests are scanned)
- Regex-based custom rules
- `.gitignore`-style exclusion config

---

## Success Criteria

1. `aibodyguard -- opencode` works with no manual configuration
2. Secret values from discovered files are redacted in outbound requests
3. Non-secret config values (e.g., `APP_ENV=production`) are not redacted (no false positives)
4. LLM streaming responses work correctly
5. Single binary, no runtime dependencies
6. Clear README with installation and usage instructions
