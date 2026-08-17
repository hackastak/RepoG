# Chunking

RepoG splits repository content into chunks before embedding it. Chunk size is not
fixed — it is derived from the token limit of whichever embedding model you have
configured, so that chunks always fit within the model's context and no capacity is
wasted on models that could handle more.

The decision and the alternatives considered are recorded in
[ADR-004](adr/ADR-004-dynamic-chunking-based-on-model-token-limits.md).

## How chunk size is calculated

At sync time, RepoG asks the configured embedding provider for its token limit and
converts it to a character budget:

```
maxChars = floor(maxTokens × 0.90) × 3
```

- **90% of the token limit** leaves a safety margin for tokenization variance,
  special characters, and encoding overhead.
- **3 characters per token** is deliberately conservative. The real-world average is
  closer to 4:1, but code with dense symbols, URLs, and mixed languages can push
  well below that, and an over-long chunk is rejected by the API outright.

Applied to the supported models:

| Provider / model | Token limit | Chunk size |
|---|---|---|
| Gemini `gemini-embedding-2-preview` (default) | 8,192 | 22,116 chars |
| Gemini `gemini-embedding-001` | 2,048 | 5,529 chars |
| OpenAI `text-embedding-3-*` | 8,191 | 22,113 chars |
| Voyage AI `voyage-code-3` | 16,000 | 43,200 chars |
| Voyage AI `voyage-3` | 32,000 | 86,400 chars |
| Ollama `nomic-embed-text` | 8,192 | 22,116 chars |
| Ollama (other models) | 256–8,192 | 690–22,116 chars |

If the provider reports no usable limit, RepoG falls back to 25,000 characters.

To see the value being used for a given run:

```bash
repog sync --verbose
# Using chunk size: 22116 characters (based on 8192 token limit)
```

## Switching embedding providers

Chunks produced for one token limit are not reusable under a smaller one, so
changing the embedding provider or model can require re-chunking. RepoG detects this
during `repog reconfig` and asks before touching anything.

When the new model's chunk size differs from the current one:

```
Warning: Embedding configuration has changed

  Previous: openrouter (openai/text-embedding-3-small, 1536d)
  New:      gemini (gemini-embedding-001, 3072d)

  Chunk size will change:
     Previous: 22,113 characters
     New:      5,529 characters

  This will delete ALL existing embeddings AND chunks.
  You'll need to run:
    1. repog sync  (to re-chunk with new size)
    2. repog embed (to generate new embeddings)

? Continue with reconfiguration? (y/N)
```

On confirmation, RepoG clears all embeddings and chunks and resets sync state, then
you re-run `repog sync` followed by `repog embed`.

When the new model has the **same** token limit but different dimensions, chunks are
left intact and only the embeddings are cleared — `repog embed` alone is enough.
This clear-on-change strategy, and why partial migration was rejected, is covered in
[ADR-003](adr/ADR-003-clear-on-change-strategy-for-embedding-migrations.md).

### What gets cleared

| Table | Effect |
|---|---|
| `chunks` | All rows deleted (only when chunk size changes) |
| `chunk_embeddings` | Dropped and recreated at the new dimension count |
| `repos` | `pushed_at_hash`, `embedded_hash`, `embedded_at` set to `NULL` |
| `sync_state` | All rows deleted |

## Troubleshooting

**Embedding fails with "token limit exceeded."**
The chunks were most likely produced under an earlier configuration with a larger
budget. Re-select your current provider to trigger a re-chunk:

```bash
repog reconfig embedding
repog sync
repog embed
```

**"Failed to create embedding provider" during sync.**
The API key for the configured provider is missing from the keyring. Re-enter it:

```bash
repog reconfig embedding
```

**Re-syncing everything sounds slow — how long does it take?**
It is bounded by the GitHub API more than by chunking. Roughly 2–5 minutes for
50–100 repositories, 5–10 minutes for 100–200.

**I edited `~/.config/repog/config.yaml` by hand.**
Chunk size is computed at sync time from the configured provider, so a manual edit
takes effect on the next `repog sync`. It does not retroactively re-chunk what is
already stored.
