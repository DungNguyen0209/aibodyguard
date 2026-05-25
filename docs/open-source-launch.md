# Open-Source Launch Guide for AIBodyguard

This document covers everything you need to publish AIBodyguard as a public open-source project on GitHub — from repository setup to community configuration.

---

## 1. Fix the Go Module Name

Right now `go.mod` uses a placeholder module path. Before publishing, replace it with your real GitHub username.

```bash
# Replace throughout the codebase
find . -type f -name "*.go" | xargs sed -i '' 's|github.com/yourusername/aibodyguard|github.com/YOUR_GITHUB_USERNAME/aibodyguard|g'
sed -i '' 's|github.com/yourusername/aibodyguard|github.com/YOUR_GITHUB_USERNAME/aibodyguard|g' go.mod
```

Also update the install URL in `README.md` to match.

---

## 2. Create the GitHub Repository

1. Go to https://github.com/new
2. Repository name: `aibodyguard`
3. Description: `Credential leak prevention wrapper for AI coding agents (Claude Code, OpenCode, Aider)`
4. Visibility: **Public**
5. Do **not** initialise with README, .gitignore, or licence — you already have all of these
6. Click **Create repository**

Then push your existing local repo:

```bash
git remote add origin https://github.com/YOUR_GITHUB_USERNAME/aibodyguard.git
git branch -M main
git push -u origin main
git push origin feat/request-logging   # push your feature branch too
```

---

## 3. Required Repository Files

All of these already exist or need small updates:

| File | Status | Action |
|---|---|---|
| `README.md` | exists, current | none — already has usage, --test docs, tables |
| `LICENSE` | exists, MIT | update copyright year/name if needed |
| `go.mod` | exists | fix module name (step 1) |
| `.gitignore` | exists | none |
| `CONTRIBUTING.md` | missing | create (section 4) |
| `CHANGELOG.md` | missing | create (section 5) |
| `.github/ISSUE_TEMPLATE/` | missing | create (section 6) |
| `.github/PULL_REQUEST_TEMPLATE.md` | missing | create (section 6) |
| `.github/workflows/ci.yml` | missing | create (section 7) |

---

## 4. CONTRIBUTING.md

Create `.github/CONTRIBUTING.md` (GitHub auto-links it from the repo sidebar):

```markdown
# Contributing to AIBodyguard

Thank you for your interest in contributing.

## Development Setup

Requirements: Go 1.22+

```bash
git clone https://github.com/YOUR_GITHUB_USERNAME/aibodyguard.git
cd aibodyguard
go build ./...
go test ./...
```

## Running Tests

```bash
go test ./...
```

All tests must pass before submitting a PR.

## Making Changes

1. Fork the repository
2. Create a branch: `git checkout -b feat/your-feature`
3. Make your changes with tests
4. Run `go test ./...` and `go build ./...`
5. Open a pull request against `main`

## Code Style

- Follow standard Go formatting (`gofmt`)
- Add tests for any new behaviour
- Keep commits focused — one logical change per commit

## Reporting Issues

Use the GitHub Issues tab. Include:
- Your OS and Go version
- The agent you were wrapping (claude / opencode / other)
- Steps to reproduce
- Relevant lines from `/tmp/aibodyguard.log`
```

---

## 5. CHANGELOG.md

Create `CHANGELOG.md` at the repo root to track releases:

```markdown
# Changelog

All notable changes to this project will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added
- Per-tool agent config: `aibodyguard claude` and `aibodyguard opencode` inject
  tool-specific env vars (see `internal/agentconfig/`)
- `--test` flag: request logging to `/tmp/aibodyguard-requests.log` is now
  opt-in only (`aibodyguard --test claude`)
- Credential files with `setting` in filename are now parsed (YAML + JSON)
- `DiscoverSecrets` collects all distinct values per key (`map[string][]string`)
  — duplicate keys across microservice YAML files are fully captured
- Scanner deduplicates secret values across keys using a hash set

### Changed
- Redaction placeholder changed from `[REDACTED:KEY]` to `****`
- Startup banner now shows tool name and mode (normal / TEST)
```

---

## 6. GitHub Issue and PR Templates

### `.github/ISSUE_TEMPLATE/bug_report.md`

```markdown
---
name: Bug report
about: Something is not working correctly
---

**Describe the bug**
A clear description of what went wrong.

**To Reproduce**
Steps to reproduce the behaviour.

**Expected behaviour**
What you expected to happen.

**Environment**
- OS:
- Go version (`go version`):
- AIBodyguard version / commit:
- Agent being wrapped (claude / opencode / other):

**Logs**
Paste relevant lines from `/tmp/aibodyguard.log`.
```

### `.github/ISSUE_TEMPLATE/feature_request.md`

```markdown
---
name: Feature request
about: Suggest an improvement
---

**What problem does this solve?**

**Describe the solution you'd like**

**Alternatives you have considered**
```

### `.github/PULL_REQUEST_TEMPLATE.md`

```markdown
## What does this PR do?

## How to test

## Checklist
- [ ] `go test ./...` passes
- [ ] `go build ./...` passes
- [ ] New behaviour has tests
- [ ] README updated if user-facing behaviour changed
```

---

## 7. GitHub Actions CI

Create `.github/workflows/ci.yml` to run tests on every push and PR:

```yaml
name: CI

on:
  push:
    branches: [main, "feat/**"]
  pull_request:
    branches: [main]

jobs:
  test:
    name: Test
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
        go: ["1.22", "1.23"]

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}

      - name: Build
        run: go build ./...

      - name: Test
        run: go test ./...
```

---

## 8. GitHub Repository Settings

After creating the repo, configure these in **Settings**:

### General
| Setting | Value |
|---|---|
| Default branch | `main` |
| Allow merge commits | off |
| Allow squash merging | on (default) |
| Allow rebase merging | off |
| Automatically delete head branches | on |

### Branch Protection for `main`
Go to **Settings → Branches → Add rule** for `main`:

| Rule | Value |
|---|---|
| Require a pull request before merging | on |
| Require status checks to pass | on — select `Test (ubuntu-latest, 1.22)` |
| Require branches to be up to date | on |
| Do not allow bypassing the above settings | on |

### Topics (tags)
Add these in the repo About section to help people find the project:

```
golang security ai llm proxy credential-management claude opencode
```

---

## 9. First Release

Once `main` is stable, create a release:

```bash
git tag -a v0.1.0 -m "Initial public release"
git push origin v0.1.0
```

Then on GitHub go to **Releases → Draft a new release**, select `v0.1.0`, and paste a summary from `CHANGELOG.md`.

To publish pre-built binaries automatically, add a release workflow:

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags: ["v*"]

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Build binaries
        run: |
          GOOS=darwin  GOARCH=arm64 go build -o dist/aibodyguard-darwin-arm64  ./cmd/aibodyguard/
          GOOS=darwin  GOARCH=amd64 go build -o dist/aibodyguard-darwin-amd64  ./cmd/aibodyguard/
          GOOS=linux   GOARCH=amd64 go build -o dist/aibodyguard-linux-amd64   ./cmd/aibodyguard/
          GOOS=linux   GOARCH=arm64 go build -o dist/aibodyguard-linux-arm64   ./cmd/aibodyguard/
          GOOS=windows GOARCH=amd64 go build -o dist/aibodyguard-windows-amd64.exe ./cmd/aibodyguard/

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*
```

---

## 10. Checklist Before Going Public

- [ ] Fix module name in `go.mod` and all `.go` files (step 1)
- [ ] Update `LICENSE` copyright name
- [ ] Create GitHub repo as public (step 2)
- [ ] Push code
- [ ] Create `CONTRIBUTING.md`
- [ ] Create `CHANGELOG.md`
- [ ] Create issue templates and PR template
- [ ] Add CI workflow — confirm it passes
- [ ] Configure branch protection on `main`
- [ ] Add repo topics/tags
- [ ] Tag `v0.1.0` and create first release
