# Contributing to AIBodyguard

Thank you for your interest in contributing.

## Development Setup

Requires Go 1.22+.

```bash
git clone https://github.com/DungNguyen0209/aibodyguard.git
cd aibodyguard
go build ./...
go test ./...
```

## Making Changes

1. Fork the repository
2. Create a branch: `git checkout -b feat/your-feature`
3. Make your changes with tests
4. Run `go test ./...` and `go build ./...`
5. Open a pull request against `main`

## Code Style

- Standard Go formatting (`gofmt`)
- Add tests for any new behaviour
- Keep commits focused — one logical change per commit

## Reporting Issues

Use the [GitHub Issues](https://github.com/DungNguyen0209/aibodyguard/issues) tab. Include:

- Your OS and Go version (`go version`)
- The agent you were wrapping (`claude` / `opencode` / other)
- Steps to reproduce
- Relevant lines from `/tmp/aibodyguard.log`
