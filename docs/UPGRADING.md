# Upgrading RepoG

This guide covers what to expect when moving between RepoG versions. RepoG follows
[Semantic Versioning](https://semver.org/), and until `v1.0.0` breaking changes are
called out here and in the [Changelog](../CHANGELOG.md).

The short version: **your synced repositories and credentials survive upgrades.** The one
thing that can require action is re-embedding when you change embedding models — see
[Re-embedding after a model or dimension change](#re-embedding-after-a-model-or-dimension-change).

## Updating the binary

How you update depends on how you installed RepoG:

```bash
# Homebrew (macOS)
brew upgrade hackastak/tap/repog

# go install
go install github.com/hackastak/repog/cmd/repog@latest
```

Or download the newest build from the [Releases page](https://github.com/hackastak/repog/releases).

Your data is independent of the binary: the SQLite knowledge base lives at
`~/.repog/repog.db`, config at `~/.config/repog/config.yaml`, and secrets in your OS keyring
(see below). Replacing the binary never touches any of these.

## Credentials and the system keyring

API keys and your GitHub token are **never written to disk**. They are stored in your
operating system's secret store via the system keyring:

- **macOS** — Keychain
- **Windows** — Credential Manager
- **Linux** — Secret Service (GNOME Keyring, KWallet, etc.)

Only non-secret settings (chosen providers, models, dimensions, database path) live in the
plaintext `~/.config/repog/config.yaml`. Because secrets are keyed to your user account in the
OS store, they persist across upgrades and are not affected by editing or deleting the config
file. If you ever need to rotate a key or move to a new machine, use `repog reconfig` (below)
rather than hand-editing anything.

## Upgrading from v0.1.x to v0.2.x (multi-provider)

RepoG `v0.1.x` supported a single provider: Google Gemini. `v0.2.0` introduced the
multi-provider architecture (OpenAI, Anthropic, Voyage AI, OpenRouter, and local Ollama, in
addition to Gemini) and a new `repog reconfig` command.

**Your old config is migrated automatically.** On first run of a `v0.2.x`+ binary, a pre-v0.2
config is upgraded in memory to the current schema with Gemini kept as the default embedding
and generation provider — matching your previous behavior. This migration is non-destructive:
it does not rewrite your config file until you save changes, and it never touches your synced
data or embeddings.

Nothing is required to keep using Gemini. To take advantage of the new providers, run:

```bash
repog reconfig               # Reconfigure everything interactively
repog reconfig embedding     # Switch/reconfigure only the embedding provider
repog reconfig generation    # Switch/reconfigure only the generation (LLM) provider
repog reconfig github        # Update only your GitHub token
```

`reconfig` validates each credential against the live provider before saving, so you find out
immediately if a key is wrong.

### Re-embedding after a model or dimension change

Embeddings from different models are not comparable, and sqlite-vec virtual tables are created
with a fixed vector dimensionality that cannot be altered in place. RepoG therefore uses a
**clear-on-change** strategy (see [ADR-003](adr/ADR-003-clear-on-change-strategy-for-embedding-migrations.md)):

> When you change the embedding **provider**, **model**, or **dimensions**, RepoG clears all
> stored embeddings so the database is never left in a mixed, semantically invalid state.

`reconfig` warns you before this happens. After a change that clears embeddings, run:

```bash
repog embed
```

to regenerate them. Re-embedding a typical corpus takes roughly 5–10 minutes and is a one-time
cost per change. Switching only the **generation (LLM)** provider does **not** affect
embeddings and requires no re-embed. Your synced repository metadata and content are untouched
in all cases.

## Upgrading from v0.2.x to v0.3.0 (interactive TUI)

`v0.3.0` adds the interactive terminal UI: running `repog` with no subcommand on a TTY opens a
tabbed dashboard. This is purely a presentation layer over the same engine — **no data or
config migration is required**, and every existing subcommand continues to work unchanged. On
a non-TTY (pipes/CI) `repog` still falls back to printing help and never blocks for input.

If you launch the bare `repog` and have no usable config yet, the TUI drops into the same
guided setup flow as `repog init`. Existing users are unaffected.

## Downgrading

Downgrading the binary is generally safe because the on-disk database schema has been backward
compatible across `v0.x` releases. The exception is the embedding vector table: if a newer
version re-created it at a different dimensionality, an older binary expecting the previous
dimensions may need a `repog embed` to repopulate. When in doubt, re-run `repog embed` after
switching versions.
