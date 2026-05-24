# Collect All Secret Values Across Duplicate Keys Design

**Date:** 2026-05-24  
**Status:** Approved

## Overview

When the same key (e.g. `JDBC_URL`) appears in multiple config files across different microservices, the current implementation keeps only the last-seen value. This means most JDBC URLs are never redacted.

The fix:
1. `DiscoverSecrets` returns `map[string][]string` — all distinct values per key, across all files.
2. `scanner.New` accepts `map[string][]string`, flattens all values into a single hash set (`map[string]struct{}`), and uses it for O(1) dedup at construction time and O(n) iteration at redaction time.
3. `Redact` returns `(cleaned string, redactedValues []string)` — the list of actual secret values that were matched and replaced, instead of key names. This is logged in the request log's `redacted_keys` field.

---

## Data Flow

```
parser.DiscoverSecrets(root)
  → map[string][]string   e.g. {"JDBC_URL": ["jdbc:...rds1", "jdbc:...rds2"], "API_KEY": ["sk-abc"]}

scanner.New(map[string][]string)
  → flattens all values → deduplicates → stores as map[string]struct{}
    {"jdbc:...rds1": {}, "jdbc:...rds2": {}, "sk-abc": {}}

scanner.Redact(body)
  → iterates values (longest first), replaces each with ****
  → returns (cleaned, []string of matched values)
```

---

## Component Changes

### `internal/parser/discover.go`

- `DiscoverSecrets` return type: `map[string]string` → `map[string][]string`
- `Discover()` method on `fileParser` return type updated to match
- When merging parsed values into `all`:

```go
// Before (last write wins):
all[k] = v

// After (collect all distinct values per key):
existing := all[k]
for _, ev := range existing {
    if ev == v {
        // already present, skip
        goto next
    }
}
all[k] = append(all[k], v)
next:
```

Or equivalently using a per-key seen set. The result: every distinct value for a key is preserved.

### `internal/scanner/scanner.go`

- `New` signature: `New(secrets map[string]string) Scanner` → `New(secrets map[string][]string) Scanner`
- Internal storage changes from `map[string]string` to `map[string]struct{}` (hash set of values)
- Constructor:

```go
func New(secrets map[string][]string) Scanner {
    seen := make(map[string]struct{})
    for _, vals := range secrets {
        for _, v := range vals {
            seen[v] = struct{}{}
        }
    }
    return &redactScanner{values: seen}
}
```

- `Redact` iterates values sorted longest-first (same strategy as before, prevents substring collisions):

```go
func (s *redactScanner) Redact(body string) (string, []string) {
    // build sorted slice from map keys (values)
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

- `redactedValues []string` returned — these are the actual matched secret values, logged in the request log's `redacted_keys` field.

### `internal/mitm/proxy.go`

No signature change needed — `Redactor` interface stays:
```go
Redact(input string) (cleaned string, redactedKeys []string)
```
The field is now semantically "redacted values" but the interface name stays unchanged to minimize diff.

### `cmd/aibodyguard/main.go`

- Startup secret log: `secrets` is now `map[string][]string`, so the log loop changes to show all values per key:

```
[aibodyguard] discovered secrets (3 keys, 12 unique values):
[aibodyguard]   JDBC_URL (8 values):
[aibodyguard]     jdbc:mysql:aws://...rds1.../gots-db
[aibodyguard]     jdbc:mysql:aws://...rds1.../govs-db
[aibodyguard]     ...
[aibodyguard]   JWT_TOKEN (1 value):
[aibodyguard]     eyJra...
```

---

## Files Changed

| File | Change |
|------|--------|
| `internal/parser/discover.go` | Return `map[string][]string`, collect all values per key |
| `internal/scanner/scanner.go` | Accept `map[string][]string`, store values as hash set, return matched values |
| `internal/scanner/scanner_test.go` | Update tests for new signature and return semantics |
| `internal/parser/discover_test.go` | Update tests for new return type |
| `cmd/aibodyguard/main.go` | Update startup log for new map type |

---

## Out of Scope

- Changing the `Redactor` interface name (`redactedKeys` field stays named as-is)
- Persisting the per-key value mapping beyond startup logging
- Deduplicating across keys (same value under two different key names is fine — the hash set naturally deduplicates)
