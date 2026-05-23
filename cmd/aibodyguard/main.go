package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

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

	// Open log file — all mid-session output goes here, never to stderr
	logPath := filepath.Join(os.TempDir(), "aibodyguard.log")
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	logWriter := os.Stderr // fallback if file open fails
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

	// Start proxy
	s := scanner.New(secrets)
	p, err := proxy.New(s, logWriter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aibodyguard: error starting proxy: %v\n", err)
		os.Exit(1)
	}
	defer p.Shutdown()

	proxyBase := fmt.Sprintf("http://127.0.0.1:%d", p.Port())

	// Patch OpenCode config for GitHub Copilot intercept
	opencodeCfgPath := opencodeConfigPath()
	restoreFunc, patchErr := patchOpencodeConfig(opencodeCfgPath, proxyBase+"/copilot", logWriter)
	if patchErr != nil {
		fmt.Fprintf(logWriter, "[aibodyguard] warning: could not patch opencode config for Copilot: %v\n", patchErr)
	} else if restoreFunc != nil {
		defer restoreFunc()
	}

	// ── Startup banner (printed BEFORE agent starts, safe from TUI collision) ──
	copilotStatus := "active"
	if patchErr != nil {
		copilotStatus = "unavailable"
	}
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  AIBodyguard  active\n")
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Secrets loaded : %d values\n", len(secrets))
	fmt.Fprintf(os.Stderr, "  Proxy          : %s\n", proxyBase)
	fmt.Fprintf(os.Stderr, "  Copilot route  : %s\n", copilotStatus)
	fmt.Fprintf(os.Stderr, "  Log            : %s\n", logPath)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "\n")

	// Spawn the agent with injected env vars (path-prefixed URLs for routing)
	cmd := exec.Command(agentArgs[0], agentArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"ANTHROPIC_BASE_URL="+proxyBase+"/anthropic",
		"OPENAI_BASE_URL="+proxyBase+"/openai",
		"OPENAI_API_BASE="+proxyBase+"/openai",
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

	// ── Exit line (printed AFTER agent TUI has exited, safe from collision) ──
	fmt.Fprintf(os.Stderr, "\n  AIBodyguard  session ended at %s  |  log: %s\n\n",
		time.Now().Format("15:04:05"), logPath)

	os.Exit(exitCode)
}

// opencodeConfigPath returns the path to opencode.json.
func opencodeConfigPath() string {
	if configDir, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok && configDir != "" {
		return filepath.Join(configDir, "opencode", "opencode.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		return filepath.Join(home, ".config", "opencode", "opencode.json")
	}
	// Windows: %APPDATA%\opencode\opencode.json
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appData, "opencode", "opencode.json")
}

// patchOpencodeConfig injects the proxy baseURL into the github-copilot provider
// in opencode.json, and returns a function that restores the original content.
// Returns (nil, nil) if the file doesn't exist (nothing to patch).
func patchOpencodeConfig(cfgPath, copilotProxyURL string, logWriter io.Writer) (restore func(), err error) {
	if cfgPath == "" {
		return nil, nil
	}

	original, readErr := os.ReadFile(cfgPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, nil
		}
		return nil, readErr
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(original, &cfg); err != nil {
		return nil, fmt.Errorf("parse opencode.json: %w", err)
	}

	// Navigate/create: cfg["provider"]["github-copilot"]["options"]["baseURL"]
	provider, _ := cfg["provider"].(map[string]interface{})
	if provider == nil {
		provider = make(map[string]interface{})
		cfg["provider"] = provider
	}
	copilot, _ := provider["github-copilot"].(map[string]interface{})
	if copilot == nil {
		copilot = make(map[string]interface{})
		provider["github-copilot"] = copilot
	}
	options, _ := copilot["options"].(map[string]interface{})
	if options == nil {
		options = make(map[string]interface{})
		copilot["options"] = options
	}

	// Save previous value so we can restore
	prevBaseURL, hadBaseURL := options["baseURL"]
	options["baseURL"] = copilotProxyURL

	patched, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal opencode.json: %w", err)
	}

	if err := os.WriteFile(cfgPath, patched, 0644); err != nil {
		return nil, fmt.Errorf("write opencode.json: %w", err)
	}

	fmt.Fprintf(logWriter, "[aibodyguard] patched opencode.json for GitHub Copilot intercept\n")

	restore = func() {
		var restoreCfg map[string]interface{}
		if err := json.Unmarshal(original, &restoreCfg); err != nil {
			// Fall back to manual restore
			restoreProv, _ := cfg["provider"].(map[string]interface{})
			if restoreProv != nil {
				rc, _ := restoreProv["github-copilot"].(map[string]interface{})
				if rc != nil {
					ro, _ := rc["options"].(map[string]interface{})
					if ro != nil {
						if hadBaseURL {
							ro["baseURL"] = prevBaseURL
						} else {
							delete(ro, "baseURL")
						}
					}
				}
			}
			b, _ := json.MarshalIndent(cfg, "", "  ")
			os.WriteFile(cfgPath, b, 0644) //nolint:errcheck
			return
		}
		os.WriteFile(cfgPath, original, 0644) //nolint:errcheck
		fmt.Fprintf(logWriter, "[aibodyguard] restored opencode.json\n")
	}
	return restore, nil
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
