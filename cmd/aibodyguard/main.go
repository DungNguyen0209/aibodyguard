package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/yourusername/aibodyguard/internal/parser"
	"github.com/yourusername/aibodyguard/internal/proxy"
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

	// Discover secrets in current directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "aibodyguard: scanning for credential files...")
	secrets, err := parser.DiscoverSecrets(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: warning: partial scan error: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "aibodyguard: loaded %d secret values from credential files\n", len(secrets))

	// Start proxy
	s := scanner.New(secrets)
	p, err := proxy.New(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting proxy: %v\n", err)
		os.Exit(1)
	}
	defer p.Shutdown()

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", p.Port())
	fmt.Fprintf(os.Stderr, "aibodyguard: proxy listening on %s\n", proxyURL)

	// Spawn the agent with injected env vars
	cmd := exec.Command(agentArgs[0], agentArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_BASE_URL="+proxyURL,
		"OPENAI_BASE_URL="+proxyURL,
		"OPENAI_API_BASE="+proxyURL,
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

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
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
.properties), then wraps the specified agent with a local proxy that redacts any
discovered secret values before they reach the LLM API.`)
}
