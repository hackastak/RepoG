# Changelog

All notable changes to RepoG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **`ask` and `summarize` now exit non-zero when the model call fails.** Both commands previously swallowed LLM failures: the error text was written into the answer or summary body and the process exited `0`, so a rate limit or an expired key was indistinguishable from a real response and scripts had no way to detect it. `internal/ask` and `internal/summarize` now return a wrapped error, and `provider.LLMError` implements the `error` interface, so callers can recover the HTTP status with `errors.As` — enough to tell a retryable 429 from a fatal 401. `summarize` carried the same swallow on its chunk-query path, which is fixed as well. **Behavior change for scripting:** these commands now exit `1` on model failure.
- **Cited sources are ranked instead of arbitrary.** The repositories listed alongside an `ask` answer were chosen by truncating a range over a Go map, so the five shown were an arbitrary subset that varied between runs rather than the closest matches. Sources are now sorted by similarity (descending, with a repository-name tiebreak for determinism) before being capped at five.
- **`recommend` errors retain their status code.** LLM failures were re-wrapped as a bare string, discarding the HTTP status; they now wrap the underlying `*provider.LLMError` with `%w`.

### Removed

- **Dead `internal/gemini` package.** Removed 1,448 lines across five files, superseded by the self-registering `internal/provider/gemini` subpackage introduced with multi-model support ([ADR-005](docs/adr/ADR-005-factory-pattern-with-self-registration-for-providers.md)). The package had no importers; two Gemini packages under `internal/` left it ambiguous which one was live.
- **Node-era files left over from the TypeScript implementation.** Deleted `scripts/prepublish-check.sh` (a `pnpm`/`npm publish` checklist referencing a `packages/cli/package.json` that no longer exists), `.nvmrc`, and `.env.example` — RepoG reads no environment variables, since credentials come from the system keyring ([ADR-002](docs/adr/ADR-002-use-system-keyring-for-credential-storage.md)). Also untracked the committed `coverage.out` build artifact.
- **Point-in-time status reports at the repository root.** Deleted `MULTI_MODEL_IMPLEMENTATION_SUMMARY.md` and `MULTI_MODEL_TEST_PLAN.md`. Both described a four-provider snapshot (RepoG now supports six) and duplicated material already covered by [ADR-003](docs/adr/ADR-003-clear-on-change-strategy-for-embedding-migrations.md), [ADR-005](docs/adr/ADR-005-factory-pattern-with-self-registration-for-providers.md), the README, and [`docs/UPGRADING.md`](docs/UPGRADING.md).

### Documentation

- **Upgrade guide.** Added [`docs/UPGRADING.md`](docs/UPGRADING.md) covering version-to-version upgrade notes: keyring/credential behavior, the `reconfig` workflow introduced in v0.2.0, the automatic v0.1.x config migration, and the clear-on-change re-embed triggered by an embedding model or dimension change ([ADR-003](docs/adr/ADR-003-clear-on-change-strategy-for-embedding-migrations.md)). Linked from the README.
- **README architecture section.** Added a data-flow diagram covering sync → embed → query, a package responsibility table, a worked example of the provider self-registration pattern, and a table linking the ADRs behind the main design decisions. Also added a short statement of why the project exists — local-first indexing, versus string-matching repository search or uploading source to a hosted service.
- **Chunking guide moved into `docs/`.** `DYNAMIC_CHUNKING.md` is now [`docs/chunking.md`](docs/chunking.md), rewritten as reference documentation: the `floor(maxTokens × 0.90) × 3` formula and the reasoning behind both constants, per-model chunk sizes, exactly which tables are cleared when the embedding model changes, and troubleshooting. Linked from the README, and from ADR-003 and ADR-004, whose links to the old root-level path are corrected.
- **`CLAUDE.md` refreshed against the current codebase.** The package list omitted `internal/provider/` and its six vendor subpackages, `internal/tui/`, and `internal/status/`; embedding dimensions were listed as a fixed 768 rather than 768–3072 depending on model; and it claimed the `commands/` package had no tests, which has not been true since the mocked-API harness landed. Also documented provider self-registration, per-provider batch sizing, dynamic chunking, and the channel-based pipelines shared by the CLI and TUI.
- **Go version references corrected** from 1.22+ to 1.25+ in the README, `CONTRIBUTING.md`, and `AGENTS.md`, matching the `go 1.25.0` directive in `go.mod`.

### Tests & Tooling

- **Go rewrite remediation — test coverage.** Added white-box unit tests for the six provider subpackages (`internal/provider/{anthropic,gemini,openai,openrouter,ollama,voyageai}`), each previously at 0%, using `httptest`-mocked HTTP servers (no network). Per-package coverage: anthropic 90.7%, openrouter 89.0%, ollama 89.0%, voyageai 85.5%, gemini 82.3%, openai 78.5%. **Total statement coverage rose from 34.9% to 54.4%.** This is still below the 80% v1.0 target — the remaining gap is concentrated in the untested `cmd/repog` and `commands/` packages (0%) and the lower-covered `internal/provider` core (18.1%), `config` (52.1%), and `tui` (58.9%).
- **Go rewrite remediation — command tests.** Added a mocked-API test harness for the previously 0%-covered `commands/` package (`httptest` GitHub and Ollama servers, an in-memory keyring, and an isolated config dir — no network or real credentials). Covers the `status`, `sync`, and `embed` command flows end-to-end plus the `clearEmbeddings`/`clearChunks` reconfig helpers, command registration, and `maskSecret`. `commands/` rose from 0% to ~11%, bringing **total statement coverage to 57.1%**. The interactive provider-selection flows in `init`/`reconfig` remain the largest untested surface.
- **Go rewrite remediation — lint.** Resolved all outstanding `golangci-lint` findings (`golangci-lint run` is now clean at 0 issues). The original 33 `errcheck` violations were already down to 2 (unchecked `rows.Close()` in `internal/status` and `internal/tui`); these plus a `gofmt`, three `staticcheck` (QF1012), and one `unused` field are now fixed.
- **Lint now runs in CI.** A `.golangci.yml` had been in the repository since the rewrite, but no workflow job ever invoked it, so lint regressions could only be caught by running it locally. Added a `lint` job (`golangci-lint-action@v7`, matching the v2 config format) and made the `release` job depend on it as well as `test`.
- **CI builds and tests on Go 1.25.** The workflow pinned Go 1.22 against a module declaring `go 1.25.0`, which only worked because `GOTOOLCHAIN=auto` silently downloaded the newer toolchain on every run. Both the test matrix and the release job now request 1.25 directly.
- **Coverage is measured over every package that has tests.** The coverage step previously measured a curated subset that omitted the two largest packages in the project — `commands/` (2,583 statements, 10.8% covered) and `internal/tui/` (3,205 statements, 58.9%) — along with `internal/status`. Measured over that narrower set the project reports **76.2%**; measured over every package with tests it reports **48.0%**. The package list now includes `commands`, `internal/status`, and `internal/tui`, and the failing threshold moved from 50% to 45% to sit just below the honest figure. This tightens the gate rather than relaxing it: the old 50% threshold carried roughly 26 points of slack against the subset it measured, where the new one carries about 3. The 80% target for v1.0 is unchanged and remains tracked in `ROADMAP.md`. Note that the 34.9% / 54.4% / 57.1% figures recorded above were measured on a different basis and are not directly comparable to either number here.
- **Tests for the embedding provider wrapper.** `maxTokensOverrideWrapper`, which applies a configured token-limit override on top of any embedding provider, had no test coverage at all. Added tests asserting that the override takes effect, that the model default survives when no override is configured, and that every other method — including argument and result pass-through — still reaches the wrapped provider. This pins the behavior after seven hand-written forwarding methods were removed in favor of Go's method promotion from the embedded interface.

## [0.3.0] - 2026-06-18

This release ships the interactive TUI as its sole deliverable. Full code base syncing (deep clone-based ingestion) was split out to a future release; see [ADR-010](docs/adr/ADR-010-use-bubbletea-for-the-tui.md).

### Added

- **Interactive TUI** — running `repog` with no subcommand on a TTY now opens a full-screen, tabbed dashboard built with Bubbletea/Lipgloss/Bubbles (see [ADR-010](docs/adr/ADR-010-use-bubbletea-for-the-tui.md)). Switch primary views with `1`–`5` or `tab`; open Settings with `S`. Implemented so far:
  - **Repos** — multi-selectable table of indexed repositories with per-repo actions on the focused row: `s` streams an AI summary token-by-token, `r` recommends related repositories (the repo itself is dropped from its own results); both open a scrollable result pane that `esc` dismisses
  - **Status** — knowledge-base statistics and GitHub rate limit
  - **Search** — semantic search with a query box and scrollable results
  - **Ask** — RAG-based Q&A with a question box and an answer that streams in token-by-token, citing the repositories it drew from
  - **Sync/Embed** — trigger a GitHub sync (`s`), an embedding pass (`e`), or both back-to-back (`a`), with live progress streamed from the existing ingest/embed event channels and a scrollable activity log
  - **Settings & first-run setup** — a guided credential flow (GitHub token → embedding provider → generation provider → database path) that validates each credential against the live provider before saving to the keyring/config, mirroring `repog init`/`reconfig`. Launching the bare `repog` on a TTY with no usable config drops straight into this flow; once configured it is reachable any time with `S` to reconfigure. Not a numbered tab (ADR-010 UX decision #3).
- On a non-TTY (pipes/CI) `repog` continues to fall back to CLI/help and never blocks waiting for input; `NO_COLOR` is honored. Every existing subcommand keeps working unchanged — the TUI is a presentation layer over the same `internal/*` packages.

### Fixed

- TUI streaming views (Ask, Sync/Embed) now keep advancing after the user switches tabs. Pipeline/token events are routed to the view that owns them rather than to whichever tab is active, so a long-running sync or a streaming answer no longer stalls when you navigate away and back.

## [0.2.4] - 2026-04-27

### Fixed

- Fixed bug where "Use the same API key for generation?" prompt was using the previously saved key from keyring instead of the just-entered embedding key
- Fixed same issue in `reconfig` command when switching both providers to the same new provider

### Changed

- `repog init` now directs existing users to `reconfig` command instead of duplicating reconfiguration logic
- "Use the same API key?" prompt now only appears when embedding and generation providers match

## [0.2.3] - 2026-04-19

### Fixed

- Fixed handling of partitioned chunks during embedding
- Added GitHub token rotation support for long-running sync operations

## [0.2.2] - 2026-04-06

### Fixed

- Fixed version display in `repog --version` by injecting version via ldflags

## [0.2.1] - 2026-04-06

### Fixed

- Configured releases for macOS-only Homebrew distribution

## [0.2.0] - 2026-04-01

Major release introducing multi-provider support for embeddings and LLM generation.

### Added

- **Multi-Provider Support**
  - OpenAI embeddings (`text-embedding-3-small`, `text-embedding-3-large`, `text-embedding-ada-002`)
  - OpenAI LLM (`gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-3.5-turbo`)
  - Anthropic LLM (`claude-sonnet-4-20250514`, `claude-3-5-sonnet-20241022`, `claude-3-haiku-20240307`)
  - Voyage AI embeddings (`voyage-3`, `voyage-3-lite`, `voyage-code-3`)
  - OpenRouter LLM (access to 100+ models via unified API)
  - Ollama local embeddings (`nomic-embed-text`, `mxbai-embed-large`, `all-minilm`, `snowflake-arctic-embed`)
  - Ollama local LLM (Llama, Mistral, Qwen, DeepSeek, Gemma, and more)

- **New Commands**
  - `repog reconfig` - Change embedding/LLM providers without losing synced data

- **Dynamic Chunking**
  - Automatic chunk size calculation based on model token limits
  - Custom max token limit option during `init` and `reconfig`
  - Model-specific token limits and dimensions for all embedding providers

- **Enhanced Sync**
  - Default behavior syncs both owned and starred repos when no flags specified

### Changed

- Provider abstraction layer for pluggable embedding and LLM backends
- Interactive model selection with fallback options during provider changes
- Improved chunking strategy to avoid embedding API token limit errors

### Fixed

- Chunking strategy edge cases that caused embedding errors
- Go linter version compatibility issues in CI

## [0.1.0] - 2025-03-17

Initial public release of RepoG, rewritten in Go.

### Added

- **CLI Commands**
  - `repog init` - Interactive setup with credential validation
  - `repog sync` - Sync owned and/or starred repositories from GitHub
  - `repog embed` - Generate vector embeddings for synced repositories
  - `repog search` - Semantic search across your codebase
  - `repog ask` - Natural language Q&A with RAG
  - `repog recommend` - Find repositories relevant to a task
  - `repog summarize` - AI-generated repository summaries
  - `repog status` - View knowledge base statistics and API quota

- **Core Features**
  - Local SQLite database with sqlite-vec for vector storage
  - Google Gemini integration for embeddings (`gemini-embedding-2-preview`, 768 dimensions)
  - Google Gemini LLM for Q&A, recommendations, and summaries
  - Secure credential storage via system keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
  - GitHub API rate limit handling

- **Distribution**
  - Homebrew tap (`brew install hackastak/tap/repog`)
  - Pre-built binaries for macOS (amd64, arm64) and Linux (amd64, arm64)
  - Install from source via `go install`

- **Developer Experience**
  - CI pipeline with test coverage requirements
  - GoReleaser for automated releases

[Unreleased]: https://github.com/hackastak/repog/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/hackastak/repog/compare/v0.2.4...v0.3.0
[0.2.4]: https://github.com/hackastak/repog/releases/tag/v0.2.4
[0.2.3]: https://github.com/hackastak/repog/releases/tag/v0.2.3
[0.2.2]: https://github.com/hackastak/repog/releases/tag/v0.2.2
[0.2.1]: https://github.com/hackastak/repog/releases/tag/v0.2.1
[0.2.0]: https://github.com/hackastak/repog/releases/tag/v0.2.0
[0.1.0]: https://github.com/hackastak/repog/releases/tag/v0.1.0
