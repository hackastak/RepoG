# Dynamic Chunking Feature

## Overview

RepoG now automatically adjusts chunk sizes based on your configured embedding model's token limits. This ensures optimal performance and prevents embedding failures across different providers.

## How It Works

### Automatic Chunk Size Calculation

When you run `repog sync`, the chunk size is calculated based on your embedding provider:

```
Provider/Model                       Token Limit    Chunk Size
──────────────────────────────────────────────────────────────
OpenAI text-embedding-3-*            8,191 tokens   22,113 chars
Voyage AI voyage-code-3              16,000 tokens  43,200 chars
Voyage AI voyage-3                   32,000 tokens  86,400 chars
Gemini gemini-embedding-2-preview    8,192 tokens   22,116 chars   (default)
Gemini gemini-embedding-001          2,048 tokens    5,529 chars
Ollama nomic-embed-text              8,192 tokens   22,116 chars
Ollama (varies by model)             256–8,192 tk    690–22,116 ch
```

**Formula**: `(maxTokens * 0.90) * 3 = maxChars`
- Uses 90% of token limit for safety margin
- Conservative 3:1 char-to-token ratio (real-world is ~4:1)

### When Switching Providers

If you switch to a provider with a **different chunk size**, RepoG will:

1. ✅ **Detect the change** automatically
2. ⚠️ **Warn you** with full details:
   ```
   ⚠️  Warning: Embedding configuration has changed

     Previous: openrouter (openai/text-embedding-3-small, 1536d)
     New:      gemini (gemini-embedding-001, 3072d)

     ⚠️  Chunk size will change:
        Previous: 22,113 characters
        New:      5,529 characters

     This will delete ALL existing embeddings AND chunks.
     You'll need to run:
       1. repog sync  (to re-chunk with new size)
       2. repog embed (to generate new embeddings)

   ? Continue with reconfiguration? (y/N)
   ```

3. 🗑️ **Clear data** (if confirmed):
   - Delete all embeddings
   - Delete all chunks
   - Reset sync state

4. 📋 **Guide you** on next steps

## Example Scenarios

### Scenario 1: OpenAI → Gemini gemini-embedding-001 (Lower Limit)

```bash
$ ./repog reconfig embedding

# Select Gemini, model: gemini-embedding-001 (2,048 token limit)
# Chunk size changes: 22,113 → 5,529 chars
# Warning shown, data cleared

$ ./repog sync
# Re-syncs with smaller chunks

$ ./repog embed
# Generates embeddings with Gemini
```

### Scenario 2: OpenAI → Voyage AI voyage-3 (Higher Limit)

```bash
$ ./repog reconfig embedding

# Select Voyage AI, model: voyage-3
# Chunk size changes: 22,113 → 86,400 chars
# Warning shown, data cleared

$ ./repog sync
# Re-syncs with MUCH larger chunks (more efficient!)

$ ./repog embed
# Generates embeddings with Voyage AI
```

### Scenario 3: Same Provider, Different Model

If the new model has the **same token limit**, no warning is shown:
- Only embeddings are cleared (different dimensions)
- Chunks remain intact
- Just run `repog embed`

## Benefits

✅ **No more embedding failures** - Chunks always fit within token limits
✅ **Optimal efficiency** - Uses maximum safe size for each model
✅ **Automatic optimization** - No manual calculation needed
✅ **Safe migrations** - Clear warnings before destructive changes
✅ **Future-proof** - Works with any provider you add

## Technical Details

### Chunk Size Formula Rationale

**Why 90% of token limit?**
- Provides safety margin for tokenization variance
- Accounts for special characters and formatting
- Prevents edge cases near the limit

**Why 3 chars per token?**
- Conservative estimate (real-world is ~4 chars/token)
- Handles worst-case scenarios:
  - Code with lots of symbols
  - URLs and special characters
  - Mixed languages
- Better safe than failing embeddings

### Database Impact

When chunk size changes:
- `chunks` table: All rows deleted
- `chunk_embeddings` table: Dropped and recreated
- `repos` table: `pushed_at_hash`, `embedded_hash`, `embedded_at` set to NULL
- `sync_state` table: All rows deleted

This forces a complete re-sync and re-embedding with the new chunk size.

## FAQ

**Q: What if I don't want to re-sync everything?**
A: If you're switching providers but both have the same token limit (e.g., OpenAI → another 8K model), chunks won't be cleared. Only embeddings are regenerated.

**Q: How long does re-syncing take?**
A: Depends on repo count and size. With GitHub API rate limits:
- ~50-100 repos: 2-5 minutes
- ~100-200 repos: 5-10 minutes

**Q: Can I check chunk size without syncing?**
A: Yes! Run `repog sync --verbose` and it shows:
```
Using chunk size: 22116 characters (based on 8192 token limit)
```

**Q: What if I manually change the config file?**
A: The chunk size is calculated at sync time based on the configured provider. Manual config changes take effect on the next sync.

## Troubleshooting

**Issue**: Embedding fails with "token limit exceeded"
**Solution**: Your chunks might be from an older version. Run:
```bash
./repog reconfig embedding
# Re-select your current provider
# This will trigger chunk clearing and re-sync
```

**Issue**: "Failed to create embedding provider" during sync
**Solution**: Your API key might be missing. Run:
```bash
./repog reconfig embedding
# Re-enter your API key
```
