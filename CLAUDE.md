# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build binary
go build -o repog ./cmd/repog

# Install locally
go install ./cmd/repog

# Run all tests
go test ./...

# Run tests with race detection and coverage
go test -race -coverprofile=coverage.out ./...

# Run a single test file
go test ./internal/db/...

# Run a specific test by name
go test ./internal/db/... -run TestOpen

# View coverage report
go tool cover -html=coverage.out

# Lint (requires golangci-lint)
golangci-lint run
```

**CGO Requirement**: This project uses CGO for SQLite and sqlite-vec. Ensure `CGO_ENABLED=1` and a C compiler (GCC/Clang) is available.

## Architecture

RepoG is a CLI tool that syncs GitHub repositories to a local SQLite database with vector search capabilities via sqlite-vec.

### Package Structure

- `cmd/repog/` - Entry point, calls `commands.Execute()`
- `commands/` - Cobra CLI command implementations (init, sync, embed, search, ask, recommend, summarize, reconfig, status)
- `internal/` - Core business logic packages:
  - `tui/` - Bubble Tea TUI; a presentation layer over the same engine the CLI uses
  - `config/` - Configuration loading and keyring credential management
  - `db/` - SQLite database with sqlite-vec extension, schema, and migrations
  - `github/` - GitHub API client for fetching repositories
  - `provider/` - Provider abstraction (`EmbeddingProvider` / `LLMProvider`) plus one self-registering subpackage per vendor: `gemini/`, `openai/`, `anthropic/`, `voyageai/`, `openrouter/`, `ollama/`
  - `sync/` - Repository content ingestion into database
  - `embed/` - Embedding pipeline for generating and storing vectors
  - `search/` - Vector similarity search using sqlite-vec
  - `ask/` - RAG-based Q&A using retrieved chunks
  - `recommend/` - Repository recommendation engine
  - `summarize/` - AI-powered repository summarization
  - `status/` - Knowledge-base statistics collection
  - `format/` - Output formatting utilities

### Data Flow

1. `repog init` - Stores GitHub PAT and provider API keys in system keyring, config in `~/.config/repog/config.yaml`
2. `repog sync` - Fetches repo metadata from GitHub API, chunks it to fit the embedding model's token limit, stores in SQLite at `~/.repog/repog.db`
3. `repog embed` - Generates embeddings via the configured provider (768–3072 dims depending on model), stores in sqlite-vec virtual table
4. `repog search/ask/recommend` - Queries vector embeddings for similarity, uses the configured LLM provider for responses

Running `repog` with no subcommand on a TTY opens the TUI, which drives the same
`internal/` packages as the subcommands.

### Key Patterns

- Credentials stored in system keyring via `zalando/go-keyring`, never on disk
- Database uses WAL mode with sqlite-vec extension for vector operations
- Providers self-register from `init()`; the core never imports a vendor package directly (see ADR-005)
- Embeddings use `RETRIEVAL_DOCUMENT` task type for indexing, `RETRIEVAL_QUERY` for search
- Batch embedding API calls sized per provider (`BatchSize()`; 20 for Gemini, capped at 100)
- Chunk size is derived from the embedding model's token limit, not fixed (see `docs/chunking.md`)
- Long-running pipelines (`sync`, `embed`) return `<-chan Event` instead of printing, so CLI and TUI share them

## Strict Constraints

- **NO AUTOMATIC COMMITS**: Do not execute `git commit`. You may stage changes with `git add`, but the user must commit manually.
