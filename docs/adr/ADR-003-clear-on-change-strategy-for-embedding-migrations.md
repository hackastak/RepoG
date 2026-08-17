# ADR-003 Clear-on-Change Strategy for Embedding Migrations

---

## Status

`Decided`

---

## Context

RepoG supports multiple embedding providers (Gemini, OpenAI, OpenRouter, Ollama, Voyage AI) with different embedding dimensions (768, 1536, 3072). When users switch providers or models:

**What changes:**
- Provider: `gemini` → `openai`
- Model: `text-embedding-3-small` → `text-embedding-3-large`
- Dimensions: `768` → `3072`

**Problem:** Existing embeddings in the database are incompatible with the new configuration.

**Why it matters:**
- Vector dimensions must match for similarity search to work
- Mixing embeddings from different models produces semantically meaningless results
- Database schema (virtual table) is created with fixed dimensions

**Constraints:**
- SQLite virtual tables (`vec0`) cannot be altered after creation
- Vector similarity requires identical dimensionality
- Users expect search to "just work" after reconfiguration
- Must handle three scenarios:
  1. Same provider, same dimensions (just model name change)
  2. Different provider, different dimensions
  3. Different provider, same dimensions

**Assumptions:**
- Users switching providers is relatively rare (not daily)
- Re-embedding time is acceptable (5-10 minutes for typical repo count)
- Users value correctness over gradual migration complexity
- Clear is better than partial/inconsistent state

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| User Experience | High | Should be obvious what state the system is in |
| Data Integrity | High | Search results must always be semantically valid |
| Migration Time | Medium | Re-embedding overhead acceptable if rare |
| Implementation Complexity | High | Simpler is better for maintainability |
| Flexibility | Low | Nice to have progressive migration |

---

## Options

### Option A: Clear All Embeddings on Any Change (Chosen)

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| User Experience | ★★★ | Clear, predictable: empty or complete, never partial |
| Data Integrity | ★★★ | Impossible to have incompatible embeddings |
| Migration Time | ★☆☆ | Must re-embed entire corpus |
| Implementation Complexity | ★★★ | Simple: DROP TABLE, CREATE TABLE, clear flags |
| Flexibility | ★☆☆ | All-or-nothing approach |

**Trade-offs:**
- ✅ System is always in a consistent state (empty or complete)
- ✅ No risk of mixing incompatible embeddings
- ✅ Simple to implement and understand
- ✅ Clear user feedback ("You need to run `repog embed`")
- ❌ Must re-embed entire corpus (5-10 min for 100 repos)
- ❌ No partial availability during migration

**Implementation:**
```sql
-- On dimension change:
DROP TABLE chunk_embeddings;
CREATE VIRTUAL TABLE chunk_embeddings USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding FLOAT[{new_dimensions}]
);
UPDATE repos SET embedded_hash = NULL, embedded_at = NULL;
```

---

### Option B: Progressive Migration with Per-Chunk Metadata

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| User Experience | ★☆☆ | Confusing partial state, inconsistent search results |
| Data Integrity | ★★☆ | Risk of mixing embeddings if not careful |
| Migration Time | ★★★ | Can re-embed gradually, search works during migration |
| Implementation Complexity | ★☆☆ | Complex: track provider/model per chunk, filter queries |
| Flexibility | ★★★ | Supports gradual migration |

**Trade-offs:**
- ✅ Search remains functional during migration (with partial data)
- ✅ Can prioritize important repos for re-embedding
- ✅ Lower perceived downtime
- ❌ Search results inconsistent during migration (mixes old/new)
- ❌ Complex implementation: metadata tracking, query filtering
- ❌ Risk of bugs leaving orphaned embeddings
- ❌ Confusing UX: "Why are results weird?"

**Implementation:**
```sql
-- Add metadata column:
ALTER TABLE chunk_embeddings ADD COLUMN provider_model TEXT;

-- Filter queries:
SELECT * FROM chunk_embeddings
WHERE provider_model = 'openai:text-embedding-3-small';

-- Gradually re-embed and update metadata
```

---

### Option C: Dual-Index Support During Migration

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| User Experience | ★★☆ | Seamless search, but confusing setup |
| Data Integrity | ★★★ | Both old and new embeddings coexist safely |
| Migration Time | ★★☆ | Migration time same, but search never breaks |
| Implementation Complexity | ★☆☆ | Very complex: maintain two indexes, union queries |
| Flexibility | ★★★ | Full flexibility during migration |

**Trade-offs:**
- ✅ Search never breaks (queries both indexes)
- ✅ Can migrate gradually without downtime
- ✅ Safe rollback (keep old index until complete)
- ❌ Extremely complex: dual tables, union queries, storage overhead
- ❌ Confusing which index is "canonical"
- ❌ 2x storage during migration
- ❌ Query complexity increases (must union results)

**Implementation:**
```sql
-- Maintain two tables:
CREATE TABLE chunk_embeddings_old ...;
CREATE TABLE chunk_embeddings_new ...;

-- Union queries:
SELECT * FROM (
  SELECT * FROM chunk_embeddings_old
  UNION ALL
  SELECT * FROM chunk_embeddings_new
) ORDER BY distance LIMIT 10;
```

---

## Decision

We chose **Option A (Clear All Embeddings)** because it scores highest against our top priorities — user experience, data integrity, and implementation complexity — and its migration time is acceptable given:

1. **Clear mental model**: System is either "ready" or "needs embedding" — never confusing partial state
2. **Guarantees correctness**: Impossible to mix incompatible embeddings or get semantically invalid results
3. **Simple implementation**: ~20 lines of code vs. hundreds for progressive migration
4. **Rare event**: Provider switching happens infrequently (during setup or major changes)
5. **Acceptable overhead**: Re-embedding 100 repos takes ~5-10 minutes, comparable to initial setup
6. **Explicit user control**: Users confirm before clearing, understand what will happen

The migration time trade-off is acceptable because:
- Provider switching is rare (not part of daily workflow)
- Re-embedding is automatic (`repog embed`) and can run in background
- Clear warning during `repog reconfig` prevents surprises
- Time cost (5-10 min) is low compared to confusion cost of partial state

---

## Implications

**Positives:**
- Search results are always semantically valid (never mixing models)
- Simple codebase: no metadata tracking, no dual-index logic
- Clear UX: "All embeddings cleared, run `repog embed`"
- Predictable behavior: users know exactly what state the system is in
- Easy to test: only two states (empty or complete)
- Warning system prevents accidental data loss

**Negatives / Trade-offs:**
- Must re-embed entire corpus when changing providers (5-10 min)
- Search unavailable during re-embedding period
- No partial availability or gradual migration
- Users with large repos (1000+) may face longer re-embedding times
- Cannot prioritize specific repos for re-embedding first

**Watch out for:**
- If re-embedding time becomes prohibitive (>30 min), consider:
  - Adding progress indication with ETA
  - Allowing cancellation and resumption
  - Offering "re-embed specific repos" command
- If users frequently switch providers, may need to revisit this decision
- Monitor complaints about migration time in GitHub issues

> Reference this ADR from relevant code: `// See ADR-003 for why we clear all embeddings on provider change`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| User feedback | "I'd rather wait 10 minutes than get weird search results" | Confirmed correctness > speed |
| Developer (hackastak) | Simplicity matters; progressive migration is complex | Favored simple approach |
| Claude Code | Progressive migration requires extensive metadata tracking | Highlighted implementation cost |

---

## References

- Related ADRs:
  - [[ADR-001]] - SQLite choice impacts migration options (can't ALTER virtual tables)
  - [[ADR-004]] - Dynamic chunking uses similar clear-on-change pattern
- Supporting docs:
  - [UPGRADING.md](../UPGRADING.md) - User-facing migration behavior
  - [chunking.md](../chunking.md) - Similar chunking migration strategy
- Supporting code:
  - `commands/reconfig.go` - Migration detection (`embeddingChanged`), user warning/confirmation, and the `clearEmbeddings` table-recreation logic
  - `internal/db/db.go` - `MigrateEmbeddingDimensions` table-recreation helper
  - `internal/db/migrations.go` - Initial `chunk_embeddings` table creation (`CreateChunkEmbeddingsTableSQL`)
