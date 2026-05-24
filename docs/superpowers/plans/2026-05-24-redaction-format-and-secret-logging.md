# Redaction Format + Startup Secret Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `[REDACTED:KEY_NAME]` placeholder with `****` in redacted request bodies, and log all discovered secrets (key + real value) to the diagnostic log at startup.

**Architecture:** Two independent one-file changes — `internal/scanner/scanner.go` gets a one-line placeholder change, and `cmd/aibodyguard/main.go` gets a secrets inventory log block after `Discover()`. Existing tests for the scanner are updated to match the new placeholder.

**Tech Stack:** Go stdlib only — `fmt`, `sort`.

---

### Task 1: Change redaction placeholder to `****` in scanner

**Files:**
- Modify: `internal/scanner/scanner.go:47`
- Modify: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Update the existing tests to expect `****` instead of `[REDACTED:...]`**

In `internal/scanner/scanner_test.go`, replace the two assertions that check for the old placeholder:

```go
// Remove these two assertions:
if !strings.Contains(result, "[REDACTED:DB_PASSWORD]") {
    t.Error("should contain [REDACTED:DB_PASSWORD]")
}
if !strings.Contains(result, "[REDACTED:API_KEY]") {
    t.Error("should contain [REDACTED:API_KEY]")
}
```

Replace with:

```go
// Count occurrences of **** — expect exactly 2 (one per matched secret)
count := strings.Count(result, "****")
if count != 2 {
    t.Errorf("expected 2 occurrences of ****, got %d in: %s", count, result)
}
if strings.Contains(result, "supersecret123") {
    t.Error("supersecret123 should be redacted")
}
if strings.Contains(result, "sk-abc123xyz456") {
    t.Error("sk-abc123xyz456 should be redacted")
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/scanner/... -run TestScanAndRedact -v
```

Expected: `FAIL` — test still expects `[REDACTED:...]` which is still being produced.

- [ ] **Step 3: Change the placeholder in `scanner.go`**

In `internal/scanner/scanner.go`, replace line 47:

```go
// Before:
placeholder := "[REDACTED:" + e.key + "]"

// After:
placeholder := "****"
```

- [ ] **Step 4: Run all scanner tests to confirm they pass**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/scanner/... -v
```

Expected:
```
=== RUN   TestScanAndRedact
--- PASS: TestScanAndRedact (0.00s)
=== RUN   TestScanAndRedactNoMatch
--- PASS: TestScanAndRedactNoMatch (0.00s)
PASS
```

- [ ] **Step 5: Run the full test suite to confirm nothing else broke**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./...
```

Expected: all packages pass. Note: `internal/mitm` has a test `TestProxyLogsRequest` that checks `body_redacted` contains `[REDACTED:SECRET]` — this will now fail and must be fixed.

- [ ] **Step 6: Fix `TestProxyLogsRequest` in `internal/mitm/proxy_logging_test.go`**

Find this assertion in `internal/mitm/proxy_logging_test.go`:

```go
if !strings.Contains(entry.BodyRedacted, "[REDACTED:SECRET]") {
    t.Errorf("body_redacted should contain redaction marker, got: %q", entry.BodyRedacted)
}
```

Replace with:

```go
if !strings.Contains(entry.BodyRedacted, "****") {
    t.Errorf("body_redacted should contain ****, got: %q", entry.BodyRedacted)
}
```

- [ ] **Step 7: Run the full test suite again to confirm all pass**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./...
```

Expected: all packages pass.

- [ ] **Step 8: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add internal/scanner/scanner.go internal/scanner/scanner_test.go internal/mitm/proxy_logging_test.go
git commit -m "feat: replace [REDACTED:KEY] placeholder with ****"
```

---

### Task 2: Log discovered secrets at startup

**Files:**
- Modify: `cmd/aibodyguard/main.go:63-67`

- [ ] **Step 1: Add the secrets inventory log block in `main.go`**

In `cmd/aibodyguard/main.go`, find this block (around line 63):

```go
fmt.Fprintf(logWriter, "[aibodyguard] scanning for credential files in %s...\n", cwd)
secrets, err := parser.New().Discover(cwd)
if err != nil {
    fmt.Fprintf(logWriter, "[aibodyguard] warning: partial scan error: %v\n", err)
}
```

Replace with:

```go
fmt.Fprintf(logWriter, "[aibodyguard] scanning for credential files in %s...\n", cwd)
secrets, err := parser.New().Discover(cwd)
if err != nil {
    fmt.Fprintf(logWriter, "[aibodyguard] warning: partial scan error: %v\n", err)
}

// Log all discovered secrets (keys + real values) for debugging
if len(secrets) == 0 {
    fmt.Fprintf(logWriter, "[aibodyguard] discovered secrets (0): none\n")
} else {
    fmt.Fprintf(logWriter, "[aibodyguard] discovered secrets (%d):\n", len(secrets))
    keys := make([]string, 0, len(secrets))
    for k := range secrets {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    for _, k := range keys {
        fmt.Fprintf(logWriter, "[aibodyguard]   %s = %s\n", k, secrets[k])
    }
}
```

Also add `"sort"` to the import block in `main.go` if it is not already present.

- [ ] **Step 2: Build to confirm no compile errors**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build ./cmd/aibodyguard/...
```

Expected: no output (success).

- [ ] **Step 3: Smoke test — run with a directory that has a `.env` file**

Create a temporary test env file:

```bash
echo 'MY_TEST_SECRET=supersecret999' > /tmp/test.env
```

Run aibodyguard from that directory:

```bash
cd /tmp && /Users/dhmnguyen/Documents/AIBodyguard/aibodyguard -- echo done
```

Then check the diagnostic log:

```bash
cat /var/folders/sc/wshn1_b11nb991brz7xyhvps5ll62m/T/aibodyguard.log | grep "discovered"
```

Expected output:
```
[aibodyguard] discovered secrets (1):
[aibodyguard]   MY_TEST_SECRET = supersecret999
```

- [ ] **Step 4: Run full test suite one final time**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./...
```

Expected: all packages pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add -f cmd/aibodyguard/main.go
git commit -m "feat: log discovered secrets inventory at startup"
```
