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

	"github.com/yourusername/aibodyguard/internal/agentconfig"
	"github.com/yourusername/aibodyguard/internal/mitm"
	"github.com/yourusername/aibodyguard/internal/parser"
	"github.com/yourusername/aibodyguard/internal/scanner"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		os.Exit(0)
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

	// Open log file — all mid-session output goes here, never to stderr
	logPath := filepath.Join(os.TempDir(), "aibodyguard.log")
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
	secrets, err := parser.New().Discover(cwd)
	if err != nil {
		fmt.Fprintf(logWriter, "[aibodyguard] warning: partial scan error: %v\n", err)
	}

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
	p, err := mitm.New(s, logWriter, &mitm.Config{EnableRequestLog: testMode})
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting proxy: %v\n", err)
		os.Exit(1)
	}
	defer p.Shutdown()

	// Write CA cert to temp file so child process can trust it
	caPath := filepath.Join(os.TempDir(), "aibodyguard-ca.pem")
	if err := os.WriteFile(caPath, p.CACertPEM(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error writing CA cert: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(caPath)

	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", p.Port())

	// ── Startup banner ──
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  AIBodyguard  active\n")
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Tool           : %s\n", filepath.Base(agentArgs[0]))
	fmt.Fprintf(os.Stderr, "  Secrets loaded : %d values\n", len(secrets))
	if testMode {
		fmt.Fprintf(os.Stderr, "  Mode           : TEST (request log active)\n")
		fmt.Fprintf(os.Stderr, "  Request log    : /tmp/aibodyguard-requests.log\n")
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

func printUsage() {
	fmt.Fprintln(os.Stderr, `AIBodyguard — Credential leak prevention for AI coding agents

Usage:
  aibodyguard -- <agent> [agent-args...]
  aibodyguard <agent> [agent-args...]

Examples:
  aibodyguard -- opencode
  aibodyguard -- claude
  aibodyguard -- aider --model claude-3-5-sonnet

AIBodyguard scans the current directory for credential files (.env, JSON, YAML,
.properties), starts a TLS MITM proxy, and wraps the agent with HTTPS_PROXY +
NODE_EXTRA_CA_CERTS so all outbound HTTPS traffic is intercepted and secrets
are redacted before they reach any LLM API.`)
}
