# RepoG

AI-powered knowledge base for your GitHub repositories.

[![CI](https://github.com/hackastak/repog/actions/workflows/ci.yml/badge.svg)](https://github.com/hackastak/repog/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/hackastak/repog)](https://goreportcard.com/report/github.com/hackastak/repog)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## What is RepoG?

RepoG is a CLI tool that syncs your GitHub repositories to a local knowledge base, generates vector embeddings, and enables semantic search, Q&A, and AI-powered recommendations across your entire codebase.

**Why it exists:** GitHub search matches strings, not intent — it can't answer "which of my repos already solved rate limiting?" And the repositories you starred two years ago are effectively lost the moment you forget their names. The hosted tools that *can* answer those questions want your source code on their servers. RepoG keeps the index on your machine: one SQLite file under `~/.repog/`, credentials in your OS keychain, and a provider you choose — including [Ollama](https://ollama.ai), which never sends a byte off the box. See [Architecture](#architecture) for how it's put together.

**Key Features:**
- Interactive terminal UI — run `repog` with no subcommand for a full-screen, tabbed dashboard
- Sync owned and starred repositories to a local SQLite database
- Generate vector embeddings with multiple providers (Gemini, OpenAI, Voyage AI, Ollama)
- Semantic search across all your code using natural language
- Ask questions and get AI-synthesized answers (RAG) with multiple LLM providers
- Get repository recommendations for specific tasks
- Summarize repositories with AI

## Installation

### Homebrew (macOS)

```bash
brew install hackastak/tap/repog
```

To update to the latest release later:

```bash
brew upgrade hackastak/tap/repog
```

The binary is roughly 19MB — it statically links a C SQLite build and the [sqlite-vec](https://github.com/asg017/sqlite-vec) extension via CGO (see [ADR-001](docs/adr/ADR-001-use-sqlite-with-sqlite-vec-for-vector-storage.md)), so it is larger than a typical pure-Go CLI.

### Download Binary

Download the latest release for your platform from the [Releases page](https://github.com/hackastak/repog/releases). See the [Changelog](CHANGELOG.md) for version history.

### From Source

Requires Go 1.25+ and a C compiler (GCC or Clang) for CGO.

```bash
go install github.com/hackastak/repog/cmd/repog@latest
```

## Quick Start

### 1. Get Your API Keys

You'll need a GitHub token and an API key for your chosen AI provider:

**GitHub Personal Access Token (PAT)**
1. Go to [GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens](https://github.com/settings/tokens?type=beta)
2. Create a new token with:
   - **Repository access**: All repositories (or select specific ones)
   - **Permissions**: `Contents: Read-only`, `Metadata: Read-only`

**AI Provider API Key** (choose one or more)
- [Google Gemini](https://aistudio.google.com/apikey) - Embeddings and LLM
- [OpenAI](https://platform.openai.com/api-keys) - Embeddings and LLM
- [Anthropic](https://console.anthropic.com) - LLM only
- [Voyage AI](https://dash.voyageai.com) - Embeddings only
- [OpenRouter](https://openrouter.ai/keys) - Access to 100+ models
- [Ollama](https://ollama.ai) - Local models (no API key needed)

### 2. Initialize RepoG

```bash
repog init
```

This will prompt you for your API keys and store them securely in your system keychain.

### 3. Sync Your Repositories

```bash
repog sync
```

This syncs both your owned and starred repositories by default.

### 4. Generate Embeddings

```bash
repog embed
```

### 5. Start Searching

```bash
repog search "authentication middleware"
repog ask "Which repos use PostgreSQL?"
repog recommend "building a CLI tool"
```

## Interactive TUI

Running `repog` with no subcommand on a terminal opens a full-screen, tabbed dashboard built with [Bubbletea](https://github.com/charmbracelet/bubbletea). It's a presentation layer over the same engine the CLI uses — everything below is still scriptable via the individual subcommands.

```bash
repog            # opens the TUI
```

**Tabs** (switch with `1`–`5` or `tab` / `shift+tab`):

| Tab | What it does |
|-----|--------------|
| **Repos** | Browse and multi-select indexed repositories; run per-repo actions |
| **Search** | Semantic search with a query box and scrollable results |
| **Ask** | RAG-based Q&A; the answer streams in and cites the repos it drew from |
| **Sync/Embed** | Trigger a sync, an embed, or both, with live progress |
| **Status** | Knowledge-base statistics and GitHub rate limit |

**Keys:**
- `1`–`5` / `tab` / `shift+tab` — switch tabs
- `S` — open Settings (credentials & providers); also the first-run setup flow
- `q` / `ctrl+c` — quit

**Repos tab actions:**
- `space` — toggle selection on the focused row · `a` — toggle all
- `s` — stream an AI summary of the focused repo · `r` — recommend related repos
- `esc` — dismiss the result pane · `ctrl+r` — reload

**Sync/Embed tab actions:**
- `s` — sync · `e` — embed · `a` — sync then embed back-to-back

On first launch with no saved credentials, the TUI drops straight into a guided setup flow that validates each credential before saving it — the same as `repog init`. On a non-TTY (piped output or CI), `repog` falls back to printing help and never waits for input; `NO_COLOR` is honored.

## Commands

| Command | Description |
|---------|-------------|
| `repog init` | Configure API keys and initialize the database |
| `repog sync` | Sync repository metadata and content |
| `repog embed` | Generate vector embeddings for synced repos |
| `repog search <query>` | Semantic search across your codebase |
| `repog ask <question>` | Ask questions with AI-synthesized answers |
| `repog recommend <task>` | Get repository recommendations |
| `repog summarize <repo>` | AI summary of a specific repository |
| `repog reconfig` | Update API keys or switch providers |
| `repog status` | View knowledge base statistics |

### Sync Options

```bash
repog sync                   # Sync both owned and starred (default)
repog sync --owned           # Sync only your own repositories
repog sync --starred         # Sync only starred repositories
```

### Reconfiguring

```bash
repog reconfig               # Reconfigure everything interactively
repog reconfig github        # Update only your GitHub token
repog reconfig embedding     # Switch or reconfigure the embedding provider
repog reconfig generation    # Switch or reconfigure the generation (LLM) provider
```

### Summarizing a Repository

```bash
repog summarize facebook/react   # AI summary of a synced repo (owner/repo)
```

### Checking Status

```bash
repog status                 # Knowledge-base stats and GitHub rate limit
repog status --json          # Same, as JSON for scripting
```

## Architecture

RepoG is a Go CLI over a local SQLite database. There is no server, no daemon, and no
hosted component — every command opens the same database file, does its work, and exits.

### Data flow

```
repog sync                      repog embed                  repog search / ask / recommend
─────────────                   ───────────                  ──────────────────────────────
GitHub API                      unembedded chunks            query text
    │  metadata, README,             │                            │
    │  file tree                     │ provider-sized batches     │ embed as RETRIEVAL_QUERY
    ▼                                ▼                            ▼
chunk to fit the model's        embedding provider           vector search (sqlite-vec KNN)
token limit                          │                            │
    │                                │ 768–3072 dims              │ top-k chunks
    ▼                                ▼                            ▼
chunks table  ──────────────►   chunk_embeddings             prompt assembly ──► LLM provider
(SQLite)                        (sqlite-vec virtual table)   (search / ask / recommend)
```

Chunk size is not fixed — it is derived from the configured embedding model's token limit,
so chunks always fit the model being used ([docs/chunking.md](docs/chunking.md)).

Both halves are incremental: a repo whose `pushed_at` hash is unchanged is skipped on sync,
and one whose `embedded_hash` already matches is skipped on embed.

### Packages

| Package | Responsibility |
|---|---|
| `cmd/repog` | Entry point; delegates to `commands.Execute()` |
| `commands/` | Cobra command definitions and terminal output |
| `internal/tui/` | Bubble Tea TUI — a presentation layer over the same engine the CLI uses |
| `internal/config/` | Config file (`~/.config/repog/config.yaml`) and keyring credential access |
| `internal/db/` | SQLite handle, sqlite-vec registration, schema, migrations |
| `internal/github/` | GitHub REST client; rate-limit aware (waits and retries on 429/403) |
| `internal/provider/` | Provider abstraction — `EmbeddingProvider` / `LLMProvider` interfaces, plus one subpackage per vendor |
| `internal/sync/` | Fetch → chunk → store pipeline; emits progress events on a channel |
| `internal/embed/` | Batched embedding pipeline; emits progress events on a channel |
| `internal/search/` | Vector similarity search over sqlite-vec |
| `internal/ask/` | RAG Q&A — retrieve, assemble a grounded prompt, stream the answer |
| `internal/recommend/` · `internal/summarize/` | Task-specific LLM flows over the same retrieval layer |
| `internal/status/` · `internal/format/` | Stats collection and output formatting |

The long-running pipelines (`sync`, `embed`) return a `<-chan Event` rather than printing,
which is what lets the CLI render a spinner and the TUI render a live progress view over
identical code.

### Adding a provider

Each vendor lives in its own subpackage and registers itself from `init()`, so the core
never imports it directly and adding one touches no existing file:

```go
func init() {
    provider.RegisterEmbeddingProvider("acme", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
        return NewAcmeEmbeddingProvider(apiKey, cfg.Model, cfg.Dimensions)
    })
}
```

Rationale and the import-cycle problem this solves are in [ADR-005](docs/adr/ADR-005-factory-pattern-with-self-registration-for-providers.md).

### Design decisions

The [ADR index](docs/adr/README.md) records the reasoning behind the choices that shaped
the codebase — among them:

| ADR | Decision |
|---|---|
| [ADR-001](docs/adr/ADR-001-use-sqlite-with-sqlite-vec-for-vector-storage.md) | SQLite + sqlite-vec for vector storage, rather than a vector database |
| [ADR-002](docs/adr/ADR-002-use-system-keyring-for-credential-storage.md) | Credentials in the system keyring, never on disk |
| [ADR-003](docs/adr/ADR-003-clear-on-change-strategy-for-embedding-migrations.md) | Clear-on-change when the embedding model changes, rather than partial migration |
| [ADR-004](docs/adr/ADR-004-dynamic-chunking-based-on-model-token-limits.md) | Chunk size derived from the model's token limit — see [docs/chunking.md](docs/chunking.md) |
| [ADR-010](docs/adr/ADR-010-use-bubbletea-for-the-tui.md) | Bubble Tea for the TUI |

## Data & Privacy

- **Local First**: All data is stored locally in `~/.repog/repog.db`
- **Secure Credentials**: API keys are stored in your system keychain (macOS Keychain, Windows Credential Manager, or Linux Secret Service)
- **Privacy**: Code is only sent to:
  - **GitHub API**: To fetch repository metadata and content
  - **Your configured AI provider**: To generate embeddings and AI responses. This is whichever provider(s) you set up during `repog init`/`reconfig` — one of Gemini, OpenAI, Anthropic, Voyage AI, or OpenRouter. If you use [Ollama](https://ollama.ai), embeddings and responses are generated by a model running locally and no code leaves your machine.

## GitHub API Rate Limits

RepoG respects GitHub's rate limit of 5,000 requests per hour for authenticated users. Use `repog status` to check your remaining quota.

## Roadmap

RepoG is under active development. The road to `v1.0.0` includes:

- **Full code base syncing** - Deep clone-based sync to index real source code, not just metadata and READMEs
- **Incremental syncing** - Re-sync only what changed (a `v1.0.0` gate)
- **Faster embeddings** - Better batching and parallelism
- **Export capabilities** - Generate documentation and knowledge graphs from your repos

See the full [Roadmap](ROADMAP.md) for what's planned, what's after v1.0, and what's out of scope, or the [issues page](https://github.com/hackastak/repog/issues) for day-to-day work.

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on:

- Development setup and prerequisites
- Running tests and linting
- Code style and conventions
- Submitting pull requests

### Quick Links

- [Report a Bug](https://github.com/hackastak/repog/issues/new?template=bug_report.md)
- [Request a Feature](https://github.com/hackastak/repog/issues/new?template=feature_request.md)
- [Good First Issues](https://github.com/hackastak/repog/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)

## Documentation

| Document | Description |
|----------|-------------|
| [ADR index](docs/adr/README.md) | Architecture Decision Records — why the codebase is the way it is |
| [docs/chunking.md](docs/chunking.md) | How chunk size is derived, and what changes when you switch embedding models |
| [ROADMAP.md](ROADMAP.md) | Public roadmap and the path to v1.0 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Guide for contributors |
| [UPGRADING.md](docs/UPGRADING.md) | Version-to-version upgrade and migration notes |
| [CHANGELOG.md](CHANGELOG.md) | Version history and release notes |
| [LICENSE](LICENSE) | MIT License |

## License

MIT License - see [LICENSE](LICENSE) for details.

---

Built with [sqlite-vec](https://github.com/asg017/sqlite-vec). Supports [Gemini](https://ai.google.dev/), [OpenAI](https://openai.com/), [Anthropic](https://anthropic.com/), [Voyage AI](https://voyageai.com/), [OpenRouter](https://openrouter.ai/), and [Ollama](https://ollama.ai/).
