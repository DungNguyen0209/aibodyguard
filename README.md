# AIBodyguard

A credential leak prevention wrapper for AI coding agents.

AIBodyguard sits between your coding agent (OpenCode, Claude Code, Cursor, etc.) and the LLM API. It automatically discovers credential files in your project, then intercepts and redacts any secret values before they reach the AI — without any manual configuration.

## How It Works

1. At startup, AIBodyguard scans your current directory recursively for credential files (`.env`, JSON, YAML, `.properties`)
2. It starts a local HTTP proxy and injects `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL` into the agent's environment
3. All LLM API requests from the agent route through the proxy
4. Secret values are replaced with `[REDACTED:<KEY_NAME>]` before forwarding
5. You see a warning on stderr whenever a redaction occurs

## Installation

### Download binary (macOS/Linux)

```bash
# macOS (Apple Silicon)
curl -L https://github.com/yourusername/aibodyguard/releases/latest/download/aibodyguard-darwin-arm64 -o aibodyguard
chmod +x aibodyguard
sudo mv aibodyguard /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/yourusername/aibodyguard/releases/latest/download/aibodyguard-darwin-amd64 -o aibodyguard
chmod +x aibodyguard
sudo mv aibodyguard /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/yourusername/aibodyguard/releases/latest/download/aibodyguard-linux-amd64 -o aibodyguard
chmod +x aibodyguard
sudo mv aibodyguard /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/yourusername/aibodyguard.git
cd aibodyguard
go build -o aibodyguard ./cmd/aibodyguard/
sudo mv aibodyguard /usr/local/bin/
```

Requires Go 1.22+.

## Usage

```bash
# Wrap OpenCode
aibodyguard -- opencode

# Wrap Claude Code
aibodyguard -- claude

# Wrap any agent
aibodyguard -- <agent-command> [agent-args...]
```

Run from your project root. AIBodyguard will scan the current directory for credentials.

## Supported Credential File Formats

| Format | Examples |
|--------|---------|
| `.env` | `.env`, `.env.local`, `.env.production` |
| `.properties` | `application.properties`, `config.properties` |
| JSON | `credentials.json`, `service-account.json`, `*.json` |
| YAML | `secrets.yaml`, `config.yml`, `*.yaml`, `*.yml` |

## What Gets Redacted

A value is treated as a secret if it:
- Is 8+ characters long
- Is not a common config value (`true`, `false`, `localhost`, URLs, etc.)
- Is not all digits

## Example Output

```
aibodyguard: scanning for credential files...
aibodyguard: loaded 12 secret values from credential files
aibodyguard: proxy listening on http://127.0.0.1:54231
⚠  Redacted secret: DB_PASSWORD
⚠  Redacted secret: OPENAI_API_KEY
```

## Supported Agents

Any agent that respects `ANTHROPIC_BASE_URL` or `OPENAI_BASE_URL` environment variables:

- [OpenCode](https://github.com/opencodelabs/opencode)
- [Claude Code](https://claude.ai/code)
- [Aider](https://aider.chat)
- [Continue](https://continue.dev)
- [Cursor](https://cursor.sh) (via settings)

## License

MIT
