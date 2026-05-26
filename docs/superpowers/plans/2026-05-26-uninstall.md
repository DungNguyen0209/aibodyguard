# Uninstall Feature Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `aibodyguard --uninstall` flag that removes all installed components: the model cache, temp files, and the binary itself, with a confirmation prompt.

**Architecture:** Handle `--uninstall` in `main.go` before any other logic. The uninstall logic lives in a new `internal/uninstall/uninstall.go` file. It resolves the binary path via `os.Executable()`, removes `~/.cache/aibodyguard/`, known temp files, and finally the binary. Prints a summary of what was removed.

**Tech Stack:** Go stdlib only (`os`, `path/filepath`, `bufio`, `fmt`)

---

### Task 1: Implement uninstall package

**Files:**
- Create: `internal/uninstall/uninstall.go`
- Create: `internal/uninstall/uninstall_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/uninstall/uninstall_test.go
package uninstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DungNguyen0209/aibodyguard/internal/uninstall"
)

func TestRemoveCacheDir(t *testing.T) {
	// Create a fake cache dir with files
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "aibodyguard")
	if err := os.MkdirAll(filepath.Join(cacheDir, "models"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "models", "model.onnx"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	removed, err := uninstall.RemoveCacheDir(cacheDir)
	if err != nil {
		t.Fatalf("RemoveCacheDir: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("expected cache dir to be gone")
	}
}

func TestRemoveCacheDirMissing(t *testing.T) {
	removed, err := uninstall.RemoveCacheDir("/nonexistent/path/aibodyguard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Error("expected removed=false when dir does not exist")
	}
}

func TestRemoveTempFiles(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "aibodyguard.log"),
		filepath.Join(dir, "aibodyguard-ca.pem"),
	}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	removed := uninstall.RemoveTempFiles(paths)
	if len(removed) != 2 {
		t.Errorf("expected 2 removed, got %d: %v", len(removed), removed)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be deleted", p)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/uninstall/... -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement uninstall package**

```go
// internal/uninstall/uninstall.go
package uninstall

import (
	"os"
)

// RemoveCacheDir removes the entire cacheDir tree.
// Returns (true, nil) if removed, (false, nil) if it didn't exist, or (false, err) on failure.
func RemoveCacheDir(cacheDir string) (bool, error) {
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return false, nil
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveTempFiles removes each path in the list if it exists.
// Returns the list of paths that were actually removed.
func RemoveTempFiles(paths []string) []string {
	var removed []string
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed = append(removed, p)
		}
	}
	return removed
}

// RemoveBinary removes the file at binaryPath.
// Returns (true, nil) if removed, or (false, err) on failure.
func RemoveBinary(binaryPath string) (bool, error) {
	if err := os.Remove(binaryPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./internal/uninstall/... -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/uninstall/
git commit -m "feat: add uninstall package"
```

---

### Task 2: Wire --uninstall into main.go

**Files:**
- Modify: `cmd/aibodyguard/main.go`

- [ ] **Step 1: Add --uninstall handling in main()**

In `cmd/aibodyguard/main.go`, add the following block immediately after the `--version` check (around line 31) and before the `--` separator parsing:

```go
if args[0] == "--uninstall" {
    runUninstall(args[1:])
    return
}
```

Then add the `runUninstall` function at the bottom of `main.go` (before `printUsage`):

```go
func runUninstall(flags []string) {
	skipConfirm := false
	for _, f := range flags {
		if f == "--yes" || f == "-y" {
			skipConfirm = true
		}
	}

	if !skipConfirm {
		fmt.Fprintf(os.Stderr, "Remove AIBodyguard and all cached data (~290MB)? [y/N] ")
		var answer string
		fmt.Fscan(os.Stdin, &answer)
		if answer != "y" && answer != "Y" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			os.Exit(0)
		}
	}

	fmt.Fprintln(os.Stderr, "Uninstalling AIBodyguard...")

	// 1. Remove model cache
	cacheDir := modelcache.DefaultCacheDir()
	if removed, err := uninstallpkg.RemoveCacheDir(cacheDir); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not remove cache: %v\n", err)
	} else if removed {
		fmt.Fprintf(os.Stderr, "  removed: %s\n", cacheDir)
	}

	// 2. Remove temp files
	tmpDir := os.TempDir()
	tempPaths := []string{
		filepath.Join(tmpDir, "aibodyguard.log"),
		filepath.Join(tmpDir, "aibodyguard-ca.pem"),
		filepath.Join(tmpDir, "aibodyguard-requests.log"),
	}
	for _, p := range uninstallpkg.RemoveTempFiles(tempPaths) {
		fmt.Fprintf(os.Stderr, "  removed: %s\n", p)
	}

	// 3. Remove the binary itself (last — so we can still print output)
	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not resolve binary path: %v\n", err)
	} else {
		if removed, err := uninstallpkg.RemoveBinary(binPath); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not remove binary %s: %v\n", binPath, err)
		} else if removed {
			fmt.Fprintf(os.Stderr, "  removed: %s\n", binPath)
		}
	}

	fmt.Fprintln(os.Stderr, "Done. AIBodyguard has been uninstalled.")
}
```

- [ ] **Step 2: Add import for uninstall package**

In the import block of `cmd/aibodyguard/main.go`, add:

```go
uninstallpkg "github.com/DungNguyen0209/aibodyguard/internal/uninstall"
```

- [ ] **Step 3: Build to verify it compiles**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build ./cmd/aibodyguard/
```

Expected: no errors.

- [ ] **Step 4: Smoke test --uninstall --yes against a temp binary**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build -o /tmp/abg-uninstall-test ./cmd/aibodyguard/
/tmp/abg-uninstall-test --uninstall --yes
```

Expected output:
```
Uninstalling AIBodyguard...
  removed: /Users/dhmnguyen/.cache/aibodyguard
  removed: /tmp/abg-uninstall-test
Done. AIBodyguard has been uninstalled.
```

Verify binary is gone:
```bash
ls /tmp/abg-uninstall-test 2>&1
```
Expected: `No such file or directory`

- [ ] **Step 5: Run all tests to confirm nothing broken**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go test ./...
```

Expected: all packages PASS. (Note: modelcache tests will pass because they don't require the cache to exist.)

- [ ] **Step 6: Commit**

```bash
git add cmd/aibodyguard/main.go
git commit -m "feat: add --uninstall flag to remove model cache, temp files, and binary"
```

---

### Task 3: Update help text

**Files:**
- Modify: `cmd/aibodyguard/main.go` — `printUsage()` function

- [ ] **Step 1: Add --uninstall to usage**

In `printUsage()`, update the usage string to include:

```go
func printUsage() {
	fmt.Fprintln(os.Stderr, `AIBodyguard — Credential leak prevention for AI coding agents

Usage:
  aibodyguard -- <agent> [agent-args...]
  aibodyguard <agent> [agent-args...]
  aibodyguard --uninstall [--yes]

Examples:
  aibodyguard -- opencode
  aibodyguard -- claude
  aibodyguard -- aider --model claude-3-5-sonnet
  aibodyguard --uninstall        # interactive confirmation
  aibodyguard --uninstall --yes  # skip confirmation (scripting)

AIBodyguard scans the current directory for credential files (.env, JSON, YAML,
.properties), starts a TLS MITM proxy, and wraps the agent with HTTPS_PROXY +
NODE_EXTRA_CA_CERTS so all outbound HTTPS traffic is intercepted and secrets
are redacted before they reach any LLM API.

Uninstall removes: ~/.cache/aibodyguard/ (model + lib, ~290MB), temp files,
and the aibodyguard binary itself.`)
}
```

- [ ] **Step 2: Build and verify help output**

```bash
cd /Users/dhmnguyen/Documents/AIBodyguard
go build -o /tmp/abg-help-test ./cmd/aibodyguard/
/tmp/abg-help-test --help
```

Expected: usage text includes `--uninstall` examples.

- [ ] **Step 3: Commit**

```bash
git add cmd/aibodyguard/main.go
git commit -m "docs: add --uninstall to help text"
```
