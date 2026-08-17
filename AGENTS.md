# RepoG Agent Guide

This guide is for AI coding agents and human developers working on the RepoG project.

## Project Overview

RepoG is a Go CLI tool for AI-powered knowledge management of GitHub repositories. It syncs repo metadata to a local SQLite database with vector search via sqlite-vec, enabling semantic search, Q&A, and recommendations.

## Development Workflow

### Prerequisites
- Go 1.25+
- C compiler (GCC/Clang) for CGO
- `golangci-lint` for linting

### Building
```bash
go build -o repog ./cmd/repog
go install ./cmd/repog
```

### Testing
```bash
# Run all tests
go test ./...

# Run with race detection and coverage
go test -race -coverprofile=coverage.out ./...

# Run tests for a specific package
go test ./internal/db/...

# Run a specific test by name
go test ./internal/db/... -run TestOpen

# View coverage
go tool cover -html=coverage.out
```

### Linting
```bash
golangci-lint run
```

## Code Style & Conventions

### Naming
- **Files/Directories**: `snake_case` (e.g., `db_test.go`)
- **Exported types/functions**: `PascalCase` (e.g., `LoadConfig`, `EmbedResult`)
- **Unexported types/functions**: `camelCase` (e.g., `configPath`, `realKeyring`)
- **Constants**: `PascalCase` for exported, `camelCase` for unexported

### Error Handling
- Return errors rather than panicking
- Wrap errors with context when appropriate
- Use sentinel errors (e.g., `ErrNotConfigured`) for expected error conditions

### Testing
- Test files use `_test.go` suffix
- Use table-driven tests where appropriate
- Mock external dependencies (keyring, HTTP clients) for unit tests

## Git Workflow

Follow Conventional Commits:
- `feat:` new features
- `fix:` bug fixes
- `docs:` documentation
- `chore:` maintenance

## Strict Constraints

**NO AUTOMATIC COMMITS**: Do not execute `git commit`. You may stage changes with `git add`, but the user must commit manually.
