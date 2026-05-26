package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/DungNguyen0209/aibodyguard/internal/agentconfig"
	"github.com/DungNguyen0209/aibodyguard/internal/detector"
	"github.com/DungNguyen0209/aibodyguard/internal/mitm"
	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
	"github.com/DungNguyen0209/aibodyguard/internal/parser"
	"github.com/DungNguyen0209/aibodyguard/internal/scanner"
	uninstallpkg "github.com/DungNguyen0209/aibodyguard/internal/uninstall"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		os.Exit(0)
	}
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Fprintf(os.Stdout, "aibodyguard %s\n", Version)
		os.Exit(0)
	}
	if args[0] == "--uninstall" {
		runUninstall(args[1:])
		return
	}

	// Find the -- separator and parse aibodyguard-own flags (before --)
	sepIdx := -1
	testMode := false
	ownArgs := args
	for i, a := range args {
		if a == "--" {
			sepIdx = i
			ownArgs = args[:i]
			break
		}
	}

	// Strip --test from own args; remaining own args become the agent command
	// when no -- separator is present.
	var filteredOwn []string
	for _, a := range ownArgs {
		if a == "--test" {
			testMode = true
		} else {
			filteredOwn = append(filteredOwn, a)
		}
	}

	var agentArgs []string
	if sepIdx >= 0 {
		agentArgs = args[sepIdx+1:]
	} else {
		agentArgs = filteredOwn
	}

	if len(agentArgs) == 0 {
		fmt.Fprintln(os.Stderr, "aibodyguard: error: no agent command specified")
		printUsage()
		os.Exit(1)
	}

	// Use per-session filenames (PID-scoped) so concurrent sessions never collide.
	pid := os.Getpid()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("aibodyguard-%d.log", pid))
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	logWriter := io.Writer(os.Stderr)
	if logErr == nil {
		logWriter = logFile
		defer logFile.Close()
	}

	// Discover secrets in current directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(logWriter, "[aibodyguard] scanning for credential files in %s...\n", cwd)

	// Init ML model cache and detector (downloads on first run if needed)
	cacheDir := modelcache.DefaultCacheDir()
	var det *detector.Detector
	if cacheErr := modelcache.EnsureReady(cacheDir); cacheErr != nil {
		fmt.Fprintf(os.Stderr, "  warning: ML model not available, using heuristic detection only\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Loading ML model...")
		var detErr error
		det, detErr = detector.New(cacheDir)
		if detErr != nil {
			fmt.Fprintf(os.Stderr, " failed (%v), using heuristic detection only\n", detErr)
			det = nil
		} else {
			fmt.Fprintf(os.Stderr, " done\n")
		}
	}
	defer func() {
		if det != nil {
			det.Close()
		}
	}()

	fmt.Fprintf(os.Stderr, "  Scanning for secrets...")
	secrets, err := parser.New().Discover(cwd, det)
	if err != nil {
		fmt.Fprintf(logWriter, "[aibodyguard] warning: partial scan error: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, " done (%d values found)\n", func() int {
		n := 0
		for _, v := range secrets {
			n += len(v)
		}
		return n
	}())

	// Log all discovered secrets (keys + real values) for debugging
	if len(secrets) == 0 {
		fmt.Fprintf(logWriter, "[aibodyguard] discovered secrets (0 keys): none\n")
	} else {
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
				fmt.Fprintf(logWriter, "[aibodyguard]   %s (1 value):\n", k)
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
	reqLogPath := filepath.Join(os.TempDir(), fmt.Sprintf("aibodyguard-%d-requests.log", pid))
	p, err := mitm.New(s, logWriter, &mitm.Config{
		EnableRequestLog: testMode,
		RequestLogPath:   reqLogPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting proxy: %v\n", err)
		os.Exit(1)
	}
	defer p.Shutdown()

	// Write CA cert to temp file so child process can trust it
	caPath := filepath.Join(os.TempDir(), fmt.Sprintf("aibodyguard-%d-ca.pem", pid))
	if err := os.WriteFile(caPath, p.CACertPEM(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error writing CA cert: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(caPath)

	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", p.Port())

	// ── Startup banner ──
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  AIBodyguard %s  active\n", Version)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Tool           : %s\n", filepath.Base(agentArgs[0]))
	fmt.Fprintf(os.Stderr, "  Secrets loaded : %d values\n", len(secrets))
	if testMode {
		fmt.Fprintf(os.Stderr, "  Mode           : TEST (request log active)\n")
		fmt.Fprintf(os.Stderr, "  Request log    : %s\n", reqLogPath)
	} else {
		fmt.Fprintf(os.Stderr, "  Mode           : normal\n")
	}
	fmt.Fprintf(os.Stderr, "  MITM proxy     : %s\n", proxyAddr)
	fmt.Fprintf(os.Stderr, "  CA cert        : %s\n", caPath)
	fmt.Fprintf(os.Stderr, "  Log            : %s\n", logPath)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "\n")

	// Spawn the agent with proxy + CA cert injected into env only
	toolName := filepath.Base(agentArgs[0])
	var toolEnv []string
	switch toolName {
	case "claude":
		toolEnv = agentconfig.ClaudeEnv(proxyAddr, caPath)
	case "opencode":
		toolEnv = agentconfig.OpenCodeEnv(proxyAddr, caPath)
	default:
		toolEnv = agentconfig.CommonEnv(proxyAddr, caPath)
	}

	cmd := exec.Command(agentArgs[0], agentArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), toolEnv...)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting agent: %v\n", err)
		os.Exit(1)
	}

	// Forward signals to child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		if cmd.Process != nil {
			cmd.Process.Signal(sig) //nolint:errcheck
		}
	}()

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// ── Exit line ──
	fmt.Fprintf(os.Stderr, "\n  AIBodyguard  session ended at %s  |  log: %s\n\n",
		time.Now().Format("15:04:05"), logPath)

	os.Exit(exitCode)
}

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

	// 2. Remove all PID-scoped temp files (logs, CA certs, request logs from all past sessions)
	tmpDir := os.TempDir()
	patterns := []string{
		filepath.Join(tmpDir, "aibodyguard-*.log"),
		filepath.Join(tmpDir, "aibodyguard-*-ca.pem"),
		filepath.Join(tmpDir, "aibodyguard-*-requests.log"),
		// legacy fixed names from older versions
		filepath.Join(tmpDir, "aibodyguard.log"),
		filepath.Join(tmpDir, "aibodyguard-ca.pem"),
		filepath.Join(tmpDir, "aibodyguard-requests.log"),
	}
	var tempPaths []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		tempPaths = append(tempPaths, matches...)
	}
	for _, p := range uninstallpkg.RemoveTempFiles(tempPaths) {
		fmt.Fprintf(os.Stderr, "  removed: %s\n", p)
	}

	// 3. Remove the binary itself (last — so we can still print output before it's gone)
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
