package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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

	// Find the -- separator
	sepIdx := -1
	for i, a := range args {
		if a == "--" {
			sepIdx = i
			break
		}
	}

	var agentArgs []string
	if sepIdx >= 0 {
		agentArgs = args[sepIdx+1:]
	} else {
		agentArgs = args
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

	// Start TLS MITM proxy
	s := scanner.New(secrets)
	p, err := mitm.New(s, logWriter, nil)
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
	fmt.Fprintf(os.Stderr, "  Secrets loaded : %d values\n", len(secrets))
	fmt.Fprintf(os.Stderr, "  MITM proxy     : %s\n", proxyAddr)
	fmt.Fprintf(os.Stderr, "  CA cert        : %s\n", caPath)
	fmt.Fprintf(os.Stderr, "  Log            : %s\n", logPath)
	fmt.Fprintf(os.Stderr, "  Request log    : /tmp/aibodyguard-requests.log\n")
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "\n")

	// Spawn the agent with proxy + CA cert injected into env only
	cmd := exec.Command(agentArgs[0], agentArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"HTTPS_PROXY="+proxyAddr,
		"https_proxy="+proxyAddr,
		"NODE_EXTRA_CA_CERTS="+caPath,
		"NODE_TLS_REJECT_UNAUTHORIZED=1",
		"SSL_CERT_FILE="+caPath,
		"REQUESTS_CA_BUNDLE="+caPath, // Python (aider, etc.)
	)

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
