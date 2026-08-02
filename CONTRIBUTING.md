# Contributing to RepoG

Thank you for your interest in contributing! RepoG is a locally-run CLI tool, so every contributor is also a user — your experience setting this up for the first time is exactly the feedback we need.

## Table of Contents

- [Getting Started](#getting-started)
- [Project Structure](#project-structure)
- [Development Workflow](#development-workflow)
- [Coding Standards](#coding-standards)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Finding Something to Work On](#finding-something-to-work-on)
- [Getting Help](#getting-help)

---

## Getting Started

### Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.25+ | https://go.dev/dl/ |
| C Compiler | GCC or Clang | Required for CGO (SQLite) |
| golangci-lint | Latest | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| Git | Any recent | You probably have this |

### 1. Fork and clone

```bash
# Fork via GitHub UI, then:
git clone https://github.com/<your-username>/repog.git
cd repog
```

### 2. Build

```bash
go build -o repog ./cmd/repog
```

### 3. Run tests

```bash
go test ./...
```

### 4. Set up for local usage (optional)

If you want to test the CLI end-to-end, you'll need API keys:

- **GitHub PAT**: https://github.com/settings/tokens?type=beta (Contents + Metadata read-only)
- **Gemini API Key**: https://aistudio.google.com/apikey

Then run `./repog init` to configure.

> **Stuck?** Open an [issue](https://github.com/hackastak/repog/issues). Setup friction is a bug — please report it.

---

## Project Structure

```
repog/
├── cmd/repog/          # Entry point (main.go)
├── commands/           # Cobra CLI commands (init, sync, embed, search, etc.)
├── internal/
│   ├── ask/            # RAG-based Q&A
│   ├── config/         # Configuration and keyring credential storage
│   ├── db/             # SQLite database, schema, migrations
│   ├── embed/          # Embedding pipeline
│   ├── format/         # Output formatting utilities
│   ├── gemini/         # Gemini API client (embeddings + LLM)
│   ├── github/         # GitHub API client
│   ├── recommend/      # Repository recommendations
│   ├── search/         # Vector similarity search
│   ├── summarize/      # Repository summarization
│   └── sync/           # Repository content ingestion
├── .github/workflows/  # CI configuration
├── .goreleaser.yaml    # Release configuration
└── go.mod              # Go module definition
```

---

## Development Workflow

### Branch naming

```
feat/short-description       # New feature
fix/short-description        # Bug fix
chore/short-description      # Tooling, deps, refactors
docs/short-description       # Documentation only
```

### Commit messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add support for private repos
fix: handle GitHub API 422 on empty repos
docs: update setup instructions
chore: bump sqlite-vec to latest
```

### Running tests

```bash
go test ./...                           # Run all tests
go test -race ./...                     # Run with race detector
go test -coverprofile=coverage.out ./...  # Generate coverage
go tool cover -html=coverage.out        # View coverage report
go test ./internal/db/...               # Run tests for a specific package
go test ./internal/db/... -run TestOpen # Run a specific test
```

When adding a feature or fixing a bug, please add or update the relevant tests.

### Linting

```bash
golangci-lint run
```

CI will fail on lint errors. Run this before pushing.

---

## Coding Standards

- **Standard Go style** — use `gofmt`; follow [Effective Go](https://go.dev/doc/effective_go)
- **Error handling** — return errors rather than panicking; wrap with context when helpful
- **No secrets in code** — credentials go in the system keyring via `internal/config`
- **Keep functions small** — if a function does more than one thing, split it
- **Comment the *why*, not the *what*** — the code shows what it does; comments explain decisions
- **Table-driven tests** — use them where you have multiple cases to test

---

## Submitting a Pull Request

1. Make sure your branch is up to date with `main`:
   ```bash
   git fetch origin
   git rebase origin/main
   ```

2. Run the full check suite locally:
   ```bash
   golangci-lint run && go test ./...
   ```

3. Open a PR against `main`. Describe what the change does and how to test it.

4. Keep PRs focused. One logical change per PR. A PR that fixes a bug and adds a feature should be two PRs.

5. If your PR is a work in progress, open it as a **Draft**.

---

## Finding Something to Work On

New here? Start with issues tagged [`good first issue`](https://github.com/hackastak/repog/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).

For bigger changes, please open an issue before writing code. It avoids situations where you spend time on something that conflicts with the roadmap or another open PR.

---

## Getting Help

- **Bug reports**: [Open an issue](https://github.com/hackastak/repog/issues/new)
- **Feature ideas**: [Open an issue](https://github.com/hackastak/repog/issues/new)
- **Questions**: [GitHub Discussions](https://github.com/hackastak/repog/discussions)

We aim to respond within a few days. Thanks for contributing!
