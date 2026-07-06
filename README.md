# RepoG

AI-powered knowledge base for your GitHub repositories.

[![CI](https://github.com/hackastak/RepoG/actions/workflows/ci.yml/badge.svg)](https://github.com/hackastak/RepoG/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/hackastak/repog)](https://goreportcard.com/report/github.com/hackastak/repog)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## What is RepoG?

RepoG is a CLI tool that syncs your GitHub repositories to a local knowledge base, generates vector embeddings, and enables semantic search, Q&A, and AI-powered recommendations across your entire codebase.

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

### Download Binary

Download the latest release for your platform from the [Releases page](https://github.com/hackastak/repog/releases). See the [Changelog](CHANGELOG.md) for version history.

### From Source

Requires Go 1.22+ and a C compiler (GCC or Clang) for CGO.

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

## Data & Privacy

- **Local First**: All data is stored locally in `~/.repog/repog.db`
- **Secure Credentials**: API keys are stored in your system keychain (macOS Keychain, Windows Credential Manager, or Linux Secret Service)
- **Privacy**: Code is only sent to:
  - **GitHub API**: To fetch repository metadata and content
  - **Your configured AI provider**: To generate embeddings and AI responses. This is whichever provider(s) you set up during `repog init`/`reconfig` — one of Gemini, OpenAI, Anthropic, Voyage AI, or OpenRouter. If you use [Ollama](https://ollama.ai), embeddings and responses are generated by a model running locally and no code leaves your machine.

## GitHub API Rate Limits

RepoG respects GitHub's rate limit of 5,000 requests per hour for authenticated users. Use `repog status` to check your remaining quota.

## Roadmap

RepoG is under active development. Here's what's coming next:

- **Full code base syncing** - Deep clone-based sync to index real source code, not just metadata and READMEs
- **Performance** - Incremental syncing and re-ranking
- **Export capabilities** - Generate documentation and knowledge graphs from your repos
- **Code analysis** - Dependency graphs, language statistics, and complexity metrics
- **Multi-platform Git support** - GitLab, Bitbucket, and self-hosted Git servers

See the [issues page](https://github.com/hackastak/repog/issues) for planned features and discussions.

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
| [CONTRIBUTING.md](CONTRIBUTING.md) | Guide for contributors |
| [CHANGELOG.md](CHANGELOG.md) | Version history and release notes |
| [LICENSE](LICENSE) | MIT License |

## License

MIT License - see [LICENSE](LICENSE) for details.

---

Built with [sqlite-vec](https://github.com/asg017/sqlite-vec). Supports [Gemini](https://ai.google.dev/), [OpenAI](https://openai.com/), [Anthropic](https://anthropic.com/), [Voyage AI](https://voyageai.com/), [OpenRouter](https://openrouter.ai/), and [Ollama](https://ollama.ai/).
