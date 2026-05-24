# Request Logging Feature Design

**Date:** 2026-05-24  
**Status:** Approved

## Overview

Add structured JSON logging of all outgoing HTTPS requests intercepted by the AIBodyguard MITM proxy. Each log entry captures the full request — URL, method, headers, original body (pre-redaction), and redacted body (post-redaction) — written as newline-delimited JSON to `/tmp/aibodyguard-requests.log`.

The goal is to give developers visibility into exactly what coding agents are sending and what AIBodyguard is redacting.

---

## Architecture

A new package `internal/logger` provides a `RequestLogger` type. The `mitmProxy` struct gains a `logger *logger.RequestLogger` field. After the request body is read and redacted in `handleTunnel`, `proxy.go` calls `p.logger.Log(entry)`. The logger owns the file handle and encodes each entry as a single JSON line (newline-delimited JSON / NDJSON).

```
[coding agent]
     │  HTTPS via CONNECT
     ▼
[mitmProxy.handleTunnel]
     │  reads body, calls scanner.Redact()
     │  calls logger.Log(RequestEntry)
     │                    │
     │                    ▼
     │           /tmp/aibodyguard-requests.log
     │
     ▼
[upstream API]
```

---

## Data Model

Each log entry is one JSON object per line:

```json
{
  "timestamp": "2026-05-24T10:23:01Z",
  "method": "POST",
  "url": "https://api.openai.com/v1/chat/completions",
  "headers": {
    "Authorization": ["Bearer sk-..."],
    "Content-Type": ["application/json"]
  },
  "body_original": "{\"model\":\"gpt-4\",\"messages\":[...]}",
  "body_redacted": "{\"model\":\"gpt-4\",\"messages\":[...]}",
  "redacted_keys": ["OPENAI_API_KEY"]
}
```

- `headers` is a map of string to string slice (matching Go's `http.Header` type).
- `body_original` and `body_redacted` are raw strings. If the body is empty, both are `""`.
- `redacted_keys` is the list of secret key names that were replaced in the body. Empty array if none.
- All headers are logged in full — no masking.

---

## Components

### `internal/logger/logger.go` (new file)

```go
type RequestEntry struct {
    Timestamp    time.Time           `json:"timestamp"`
    Method       string              `json:"method"`
    URL          string              `json:"url"`
    Headers      map[string][]string `json:"headers"`
    BodyOriginal string              `json:"body_original"`
    BodyRedacted string              `json:"body_redacted"`
    RedactedKeys []string            `json:"redacted_keys"`
}

type RequestLogger struct {
    mu  sync.Mutex
    enc *json.Encoder
    f   *os.File
}

func New(path string) (*RequestLogger, error)  // opens file in append+create mode
func (l *RequestLogger) Log(e RequestEntry) error
func (l *RequestLogger) Close() error
```

### `internal/mitm/proxy.go` changes

- Add `logger *logger.RequestLogger` field to `mitmProxy`.
- After `scanner.Redact()` call (~line 162), build a `RequestEntry` from the current request and call `p.logger.Log(entry)`.
- If `p.logger` is nil (logger failed to initialize), skip logging silently.

### `internal/mitm/mitm.go` changes

- Add `RequestLogPath string` to the proxy `Config` struct. Default: `/tmp/aibodyguard-requests.log`.
- In the `New()` factory, call `logger.New(cfg.RequestLogPath)`. On error, write warning to diagnostic log and set logger to nil (non-fatal).

### `cmd/aibodyguard/main.go` changes

- Pass `RequestLogPath: "/tmp/aibodyguard-requests.log"` when constructing proxy config (or rely on the default).

---

## Error Handling

- **Log file cannot be opened at startup:** Log a warning to `/tmp/aibodyguard.log` and set the logger to nil. The proxy continues operating normally. Request logging is non-critical.
- **`Log()` call fails (e.g., disk full):** Log the error to the diagnostic log and continue. The intercepted request is still forwarded to upstream normally.
- **Concurrent requests:** `RequestLogger` uses a `sync.Mutex` to serialize writes, ensuring log entries are not interleaved.

---

## Log File Location

| File | Content |
|------|---------|
| `/tmp/aibodyguard.log` | Existing diagnostic log (startup, redaction notices, errors) |
| `/tmp/aibodyguard-requests.log` | New request log (one JSON line per intercepted request) |

---

## Out of Scope

- Log rotation (file grows unbounded per session; acceptable for debugging use)
- Response logging (only requests are logged)
- Configurable log path via CLI flag (can be added later)
- Header masking (all headers logged in full per user preference)
