# Project Layout & Interface Design

**Date:** 2026-05-23

## Goal

Reorganise AIBodyguard to follow the golang-standards/project-layout convention and introduce explicit Go interfaces for all three internal packages (`parser`, `scanner`, `proxy`), so each package has a clear contract, its concrete type is unexported, and `main.go` depends only on interfaces.

---

## Directory Layout (target)

```
AIBodyguard/
├── cmd/
│   └── aibodyguard/
│       └── main.go               ← wires interfaces only, no business logic
├── internal/
│   ├── parser/
│   │   ├── parser.go             ← Parser interface + New() constructor
│   │   ├── discover.go           ← concrete file discovery (unexported fileParser)
│   │   ├── env.go
│   │   ├── json.go
│   │   ├── yaml.go
│   │   └── *_test.go
│   ├── scanner/
│   │   ├── scanner.go            ← Scanner interface + concrete redactScanner (unexported)
│   │   └── scanner_test.go
│   └── proxy/
│       ├── proxy.go              ← Proxy interface + concrete httpProxy (unexported)
│       └── proxy_test.go
├── build/
│   └── ci/                       ← symlink or copy of CI configs (GitHub Actions stays at .github/)
├── scripts/
│   └── build.sh                  ← local build/test helper
├── Makefile                      ← build / test / lint / release targets
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

Directories omitted (YAGNI): `api/`, `web/`, `vendor/`, `deployments/`, `examples/`, `configs/`, `third_party/`.

`.github/workflows/release.yml` stays in place — GitHub Actions requires this path.

---

## Interfaces

### `internal/parser`

```go
// Parser discovers credential key/value pairs from files rooted at a directory.
type Parser interface {
    Discover(root string) (map[string]string, error)
}

// New returns a Parser backed by the filesystem implementation.
func New() Parser
```

The existing `DiscoverSecrets(root string)` top-level function is replaced by `New().Discover(root)`. All test helpers that call `DiscoverSecrets` are updated to use the interface.

### `internal/scanner`

```go
// Scanner redacts known secret values from arbitrary text.
type Scanner interface {
    Redact(input string) (cleaned string, redactedKeys []string)
}

// New returns a Scanner loaded with the given secrets map.
func New(secrets map[string]string) Scanner
```

The concrete `Scanner` struct is renamed to `redactScanner` (unexported). The `New` constructor already exists and returns the concrete type — it is changed to return the `Scanner` interface.

### `internal/proxy`

```go
// Proxy is a local HTTP interception proxy.
type Proxy interface {
    Port() int
    Shutdown()
}

// New starts a proxy and returns it. log receives redaction event lines.
func New(s scanner.Scanner, log io.Writer) (Proxy, error)
```

The concrete `Proxy` struct is renamed to `httpProxy` (unexported). The `New` constructor is changed to return the `Proxy` interface.

---

## Makefile Targets

| Target    | Command                                          |
|-----------|--------------------------------------------------|
| `build`   | `go build -o aibodyguard ./cmd/aibodyguard`      |
| `test`    | `go test ./...`                                  |
| `lint`    | `staticcheck ./...`                              |
| `clean`   | `rm -f aibodyguard`                              |

---

## What Does NOT Change

- All business logic (redaction, file discovery, HTTP proxying) is unchanged
- Import paths: `internal/parser`, `internal/scanner`, `internal/proxy`
- Tests remain co-located with their packages
- `.github/workflows/release.yml` location is unchanged
- `opencode.json` patching logic in `main.go` is unchanged

---

## Success Criteria

1. `go build ./...` passes with zero errors
2. `go test ./...` passes with zero failures
3. `main.go` imports only the three interfaces — no concrete struct types
4. `Parser`, `Scanner`, `Proxy` interfaces each have at least one test using a mock or the concrete impl via the interface
5. `make build`, `make test` work from the project root
