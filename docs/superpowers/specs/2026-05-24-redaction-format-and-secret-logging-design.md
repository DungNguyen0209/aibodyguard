# Redaction Format + Startup Secret Logging Design

**Date:** 2026-05-24  
**Status:** Approved

## Overview

Two small targeted changes:

1. **Change redaction placeholder** — secret values in intercepted request bodies are replaced with `****` instead of `[REDACTED:KEY_NAME]`. The LLM sees `****` and cannot infer key names from the body. The request log's `redacted_keys` field still records which keys were hit.

2. **Log discovered secrets at startup** — after `parser.Discover()` runs, write each discovered key and its real value to the diagnostic log (`/tmp/aibodyguard.log`). This is for local debugging only.

---

## Change 1: Redaction Placeholder

**File:** `internal/scanner/scanner.go`

**Current behavior (line 47):**
```go
placeholder := "[REDACTED:" + e.key + "]"
```

**New behavior:**
```go
placeholder := "****"
```

The `Redact()` method signature is unchanged — it still returns `(cleaned string, redactedKeys []string)`. The `redactedKeys` slice continues to carry key names. The request log entry's `body_redacted` field will contain `****` wherever a secret appeared. The `redacted_keys` field in the log preserves full traceability of which keys were matched.

**Example:**

Before: `{"api_key": "sk-abc123"}` → `{"api_key": "[REDACTED:OPENAI_API_KEY]"}`  
After:  `{"api_key": "sk-abc123"}` → `{"api_key": "****"}`

---

## Change 2: Log Discovered Secrets at Startup

**File:** `cmd/aibodyguard/main.go`

After `parser.New().Discover(cwd)` returns, and before the proxy starts, write the full secrets inventory to `logWriter`. Keys are sorted alphabetically.

**Log format:**
```
[aibodyguard] discovered secrets (3):
[aibodyguard]   DB_PASSWORD = postgres://user:realpassword@localhost/db
[aibodyguard]   OPENAI_API_KEY = sk-abc123...
[aibodyguard]   STRIPE_SECRET = sk_live_xyz...
```

If no secrets are found:
```
[aibodyguard] discovered secrets (0): none
```

**Notes:**
- Real secret values are written — this is intentional for local debugging.
- Output goes to `logWriter` which writes to `/tmp/aibodyguard.log` (the existing diagnostic log), not the request log.
- Keys are sorted with `sort.Strings` for consistent, readable output.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/scanner/scanner.go` | Replace `[REDACTED:KEY_NAME]` placeholder with `****` |
| `cmd/aibodyguard/main.go` | Add startup secret inventory log after `Discover()` |

## Out of Scope

- Changing what `redacted_keys` contains in the request log (still key names)
- Any changes to the parser, logger, or proxy packages
- Making the placeholder configurable
