# Collect All Secret Values Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the duplicate-key problem so every distinct secret value across all config files is collected and redacted, not just the last-seen value per key.

**Architecture:** `DiscoverSecrets` returns `map[string][]string` (key → all distinct values). `scanner.New` accepts this type, flattens all values into a `map[string]struct{}` hash set for O(1) dedup, and redacts by iterating values sorted longest-first. `Redact` returns matched values (not key names) as `redactedKeys`. Startup log shows each key with all its values.

**Tech Stack:** Go stdlib — `sort`, `strings`.

---

### Task 1: Update `DiscoverSecrets` to return `map[string][]string`

**Files:**
- Modify: `internal/parser/discover.go:116-189`
- Modify: `internal/parser/discover_test.go`

- [ ] **Step 1: Update `discover_test.go` to expect `map[string][]string`**

Replace the entire `TestDiscoverSecrets` test in `internal/parser/discover_test.go`:

```go
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSecrets(t *testing.T) {
	tmp := t.TempDir()

	// .env file
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("SECRET_KEY=abc12345678\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// JSON file
	if err := os.WriteFile(filepath.Join(tmp, "creds.json"), []byte(`{"token":"json-token-xyz9876"}`), 0644); err != nil {
		t.Fatalf("failed to write creds.json: %v", err)
	}

	// YAML file
	if err := os.WriteFile(filepath.Join(tmp, "secrets.yaml"), []byte("api_key: yaml-key-qwerty123\n"), 0644); err != nil {
		t.Fatalf("failed to write secrets.yaml: %v", err)
	}

	// properties file
	if err := os.WriteFile(filepath.Join(tmp, "app.properties"), []byte("db.password=props-pass-abc456\n"), 0644); err != nil {
		t.Fatalf("failed to write app.properties: %v", err)
	}

	// node_modules should be skipped
	if err := os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755); err != nil {
		t.Fatalf("failed to create node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "node_modules", "secret.env"), []byte("SKIP=should-not-load-this\n"), 0644); err != nil {
		t.Fatalf("failed to write node_modules/secret.env: %v", err)
	}

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checkContains := func(key, want string) {
		t.Helper()
		vals, ok := secrets[key]
		if !ok {
			t.Errorf("key %q not found in secrets", key)
			return
		}
		for _, v := range vals {
			if v == want {
				return
			}
		}
		t.Errorf("key %q does not contain value %q, got: %v", key, want, vals)
	}

	checkContains("SECRET_KEY", "abc12345678")
	checkContains("token", "json-token-xyz9876")
	checkContains("api_key", "yaml-key-qwerty123")
	checkContains("db.password", "props-pass-abc456")

	if _, ok := secrets["SKIP"]; ok {
		t.Error("node_modules secret should not be loaded")
	}
}

func TestDiscoverSecrets_DuplicateKey(t *testing.T) {
	tmp := t.TempDir()

	sub1 := filepath.Join(tmp, "svc1")
	sub2 := filepath.Join(tmp, "svc2")
	if err := os.MkdirAll(sub1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub2, 0755); err != nil {
		t.Fatal(err)
	}

	// Same key, different values in two different files
	if err := os.WriteFile(filepath.Join(sub1, "secrets.yaml"), []byte("JDBC_URL: jdbc:mysql://host1/db1abc12345\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "secrets.yaml"), []byte("JDBC_URL: jdbc:mysql://host2/db2abc12345\n"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vals := secrets["JDBC_URL"]
	if len(vals) != 2 {
		t.Errorf("expected 2 values for JDBC_URL, got %d: %v", len(vals), vals)
	}

	found1, found2 := false, false
	for _, v := range vals {
		if v == "jdbc:mysql://host1/db1abc12345" {
			found1 = true
		}
		if v == "jdbc:mysql://host2/db2abc12345" {
			found2 = true
		}
	}
	if !found1 {
		t.Error("missing jdbc:mysql://host1/db1abc12345")
	}
	if !found2 {
		t.Error("missing jdbc:mysql://host2/db2abc12345")
	}
}

func TestDiscoverSecrets_DuplicateValue(t *testing.T) {
	tmp := t.TempDir()

	sub1 := filepath.Join(tmp, "svc1")
	sub2 := filepath.Join(tmp, "svc2")
	os.MkdirAll(sub1, 0755)
	os.MkdirAll(sub2, 0755)

	// Same key, same value in two files — should deduplicate
	if err := os.WriteFile(filepath.Join(sub1, "secrets.yaml"), []byte("API_KEY: sk-abc123xyz456\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "secrets.yaml"), []byte("API_KEY: sk-abc123xyz456\n"), 0644); err != nil {
		t.Fatal(err)
	}

	secrets, err := New().Discover(tmp)
	if err != nil {
		t.Fatal(err)
	}

	vals := secrets["API_KEY"]
	if len(vals) != 1 {
		t.Errorf("expected 1 deduplicated value for API_KEY, got %d: %v", len(vals), vals)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/parser/... -v 2>&1 | grep -E "PASS|FAIL|cannot"
```

Expected: compile error — `secrets["SECRET_KEY"]` no longer has type `string`.

- [ ] **Step 3: Update `DiscoverSecrets` return type and collection logic in `discover.go`**

In `internal/parser/discover.go`, change the `DiscoverSecrets` function signature and the merge loop:

```go
// Change return type on line 116:
func DiscoverSecrets(root string) (map[string][]string, error) {
	all := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		// ... existing walk logic unchanged until the merge loop ...

		for k, v := range parsed {
			if isLikelySecret(v) {
				// Append only if not already present for this key
				already := false
				for _, existing := range all[k] {
					if existing == v {
						already = true
						break
					}
				}
				if !already {
					all[k] = append(all[k], v)
				}
			}
		}
		return nil
	})

	return all, err
}
```

Also update the `Discover` method on `fileParser` to match the new return type:

```go
func (p *fileParser) Discover(root string) (map[string][]string, error) {
	return DiscoverSecrets(root)
}
```

And update the `Parser` interface in `internal/parser/parser.go` (if it exists) to return `map[string][]string`.

- [ ] **Step 4: Run parser tests to confirm they pass**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/parser/... -v 2>&1 | grep -E "PASS|FAIL"
```

Expected:
```
--- PASS: TestDiscoverSecrets (0.00s)
--- PASS: TestDiscoverSecrets_DuplicateKey (0.00s)
--- PASS: TestDiscoverSecrets_DuplicateValue (0.00s)
PASS
```

- [ ] **Step 5: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add internal/parser/discover.go internal/parser/discover_test.go
git commit -m "feat: collect all distinct values per key in DiscoverSecrets"
```

---

### Task 2: Update `scanner.New` to accept `map[string][]string` and store values as hash set

**Files:**
- Modify: `internal/scanner/scanner.go`
- Modify: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Update scanner tests**

Replace the full contents of `internal/scanner/scanner_test.go`:

```go
package scanner

import (
	"strings"
	"testing"
)

func TestScanAndRedact(t *testing.T) {
	secrets := map[string][]string{
		"DB_PASSWORD":  {"supersecret123"},
		"API_KEY":      {"sk-abc123xyz456"},
		"database.url": {"postgres://supersecret123@localhost/db"},
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"my password is supersecret123 and key sk-abc123xyz456"}]}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "supersecret123") {
		t.Error("supersecret123 should be redacted")
	}
	if strings.Contains(result, "sk-abc123xyz456") {
		t.Error("sk-abc123xyz456 should be redacted")
	}

	count := strings.Count(result, "****")
	if count != 2 {
		t.Errorf("expected 2 occurrences of ****, got %d in: %s", count, result)
	}

	matchedSet := make(map[string]bool)
	for _, v := range matched {
		matchedSet[v] = true
	}
	if !matchedSet["supersecret123"] {
		t.Error("supersecret123 should be in matched list")
	}
	if !matchedSet["sk-abc123xyz456"] {
		t.Error("sk-abc123xyz456 should be in matched list")
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched values, got %d: %v", len(matched), matched)
	}
}

func TestScanAndRedactNoMatch(t *testing.T) {
	secrets := map[string][]string{
		"DB_PASSWORD": {"supersecret123"},
	}
	var s Scanner = New(secrets)

	body := `{"messages":[{"role":"user","content":"nothing secret here"}]}`
	result, matched := s.Redact(body)

	if result != body {
		t.Error("body should be unchanged when no secrets found")
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matched values, got %d", len(matched))
	}
}

func TestScanAndRedact_DuplicateKey(t *testing.T) {
	// Same key with two different values — both should be redacted
	secrets := map[string][]string{
		"JDBC_URL": {"jdbc:mysql://host1/db1abc12345", "jdbc:mysql://host2/db2abc12345"},
	}
	var s Scanner = New(secrets)

	body := `{"url1":"jdbc:mysql://host1/db1abc12345","url2":"jdbc:mysql://host2/db2abc12345"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "host1") {
		t.Error("host1 jdbc url should be redacted")
	}
	if strings.Contains(result, "host2") {
		t.Error("host2 jdbc url should be redacted")
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched values, got %d: %v", len(matched), matched)
	}
}

func TestScanAndRedact_DeduplicatesValues(t *testing.T) {
	// Same value under two different keys — should only redact once (not double-replace)
	secrets := map[string][]string{
		"KEY_A": {"sharedSecret123"},
		"KEY_B": {"sharedSecret123"},
	}
	var s Scanner = New(secrets)

	body := `{"val":"sharedSecret123"}`
	result, matched := s.Redact(body)

	if strings.Contains(result, "sharedSecret123") {
		t.Error("sharedSecret123 should be redacted")
	}
	count := strings.Count(result, "****")
	if count != 1 {
		t.Errorf("expected 1 occurrence of ****, got %d", count)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 matched value, got %d: %v", len(matched), matched)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/scanner/... -v 2>&1 | grep -E "PASS|FAIL|cannot"
```

Expected: compile error — `New` still takes `map[string]string`.

- [ ] **Step 3: Rewrite `scanner.go`**

Replace the full contents of `internal/scanner/scanner.go`:

```go
package scanner

import (
	"sort"
	"strings"
)

// Scanner redacts known secret values from arbitrary text.
type Scanner interface {
	Redact(input string) (cleaned string, redactedValues []string)
}

// New returns a Scanner loaded with the given secrets map.
// All distinct values across all keys are collected into a hash set.
func New(secrets map[string][]string) Scanner {
	seen := make(map[string]struct{})
	for _, vals := range secrets {
		for _, v := range vals {
			if v != "" {
				seen[v] = struct{}{}
			}
		}
	}
	return &redactScanner{values: seen}
}

// redactScanner is the concrete implementation of Scanner.
type redactScanner struct {
	values map[string]struct{} // hash set of all secret values
}

// Redact scans body for any known secret values and replaces them with ****.
// Values are matched longest-first to prevent substring collisions.
// Returns the (possibly modified) body and the sorted list of matched secret values.
func (s *redactScanner) Redact(body string) (string, []string) {
	// Build sorted slice from hash set — longest value first
	vals := make([]string, 0, len(s.values))
	for v := range s.values {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		return len(vals[i]) > len(vals[j])
	})

	var matched []string
	result := body

	for _, v := range vals {
		if strings.Contains(result, v) {
			result = strings.ReplaceAll(result, v, "****")
			matched = append(matched, v)
		}
	}

	sort.Strings(matched)
	return result, matched
}
```

- [ ] **Step 4: Run scanner tests to confirm they pass**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/scanner/... -v 2>&1 | grep -E "PASS|FAIL"
```

Expected:
```
--- PASS: TestScanAndRedact (0.00s)
--- PASS: TestScanAndRedactNoMatch (0.00s)
--- PASS: TestScanAndRedact_DuplicateKey (0.00s)
--- PASS: TestScanAndRedact_DeduplicatesValues (0.00s)
PASS
```

- [ ] **Step 5: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat: scanner accepts map[string][]string and stores values as hash set"
```

---

### Task 3: Fix all callers of the changed interfaces

**Files:**
- Modify: `cmd/aibodyguard/main.go`
- Check: `internal/parser/parser.go` (if Parser interface exists)

- [ ] **Step 1: Check if `parser.go` defines a `Parser` interface and update it**

```bash
cat /Users/dhmnguyen/Documents/AIBodyguard/internal/parser/parser.go
```

If it contains `Discover(root string) (map[string]string, error)`, update it to:

```go
Discover(root string) (map[string][]string, error)
```

- [ ] **Step 2: Build to find all remaining compile errors**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build ./... 2>&1
```

Expected: compile errors in `cmd/aibodyguard/main.go` because `secrets` is now `map[string][]string` but `scanner.New` previously took `map[string]string`.

- [ ] **Step 3: Update `main.go` startup secret log and `scanner.New` call**

In `cmd/aibodyguard/main.go`, find the secrets log block and the `scanner.New` call and replace both:

```go
// Log all discovered secrets (keys + real values) for debugging
if len(secrets) == 0 {
    fmt.Fprintf(logWriter, "[aibodyguard] discovered secrets (0 keys): none\n")
} else {
    // Count total unique values
    totalVals := 0
    for _, vals := range secrets {
        totalVals += len(vals)
    }
    fmt.Fprintf(logWriter, "[aibodyguard] discovered secrets (%d keys, %d unique values):\n", len(secrets), totalVals)
    keys := make([]string, 0, len(secrets))
    for k := range secrets {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    for _, k := range keys {
        vals := secrets[k]
        if len(vals) == 1 {
            fmt.Fprintf(logWriter, "[aibodyguard]   %s (%d value):\n", k, len(vals))
        } else {
            fmt.Fprintf(logWriter, "[aibodyguard]   %s (%d values):\n", k, len(vals))
        }
        for _, v := range vals {
            fmt.Fprintf(logWriter, "[aibodyguard]     %s\n", v)
        }
    }
}

// Start TLS MITM proxy
s := scanner.New(secrets)
```

- [ ] **Step 4: Build to confirm no compile errors**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build ./... 2>&1
```

Expected: no output (success).

- [ ] **Step 5: Run the full test suite**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./... 2>&1
```

Expected: all packages pass.

- [ ] **Step 6: Commit**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
git add -f cmd/aibodyguard/main.go
git add internal/parser/parser.go 2>/dev/null; true
git commit -m "feat: wire map[string][]string through main and update startup log"
```

---

### Task 4: Smoke test

- [ ] **Step 1: Rebuild binary**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build -o aibodyguard ./cmd/aibodyguard/
echo "build ok"
```

Expected: `build ok`

- [ ] **Step 2: Run from the spr repo and check the log**

```bash
rm -f /var/folders/sc/wshn1_b11nb991brz7xyhvps5ll62m/T/aibodyguard.log
cd /Users/dhmnguyen/Documents/mya-go-spr-services
/Users/dhmnguyen/Documents/AIBodyguard/aibodyguard -- echo done 2>&1
```

- [ ] **Step 3: Check JDBC_URL now has multiple values**

```bash
grep -A 10 "JDBC_URL" /var/folders/sc/wshn1_b11nb991brz7xyhvps5ll62m/T/aibodyguard.log | head -15
```

Expected: `JDBC_URL (N values):` followed by multiple distinct `jdbc:mysql:aws://...` lines.

- [ ] **Step 4: Confirm total secret count is higher than before (was 315)**

```bash
grep "discovered secrets" /var/folders/sc/wshn1_b11nb991brz7xyhvps5ll62m/T/aibodyguard.log | tail -1
```

Expected: something like `discovered secrets (X keys, Y unique values):` where Y > 315.
