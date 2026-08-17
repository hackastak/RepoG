# ADR-009 Size-Bounded, Line-Aware Chunking for Source Code

---

## Status

`Deferred to V0.5.0` (2026-06-11) — was Draft for V0.3.0. Full code base syncing (ADR-008 clone + this chunking work) was split out of V0.3.0, which ships the TUI only ([ADR-010](./ADR-010-use-bubbletea-for-the-tui.md)); it was then rescheduled from V0.4.0 to V0.5.0 once V0.4.0 landed as provider-hardening. The analysis and Option A decision below stand and carry forward to the V0.5.0 deep-sync build. See [PRD_V0.3.0 §4.4, Open Question #3/#4](../../../My_Notes/1.%20Projects/RepoG/PRD_V0.3.0.md).

---

## Context

V0.3.0 deep sync ingests **real source files** (ADR-008). Those files must be split into chunks small enough to embed within each provider's token limit, then stored in the `chunks` table and embedded by the existing pipeline (`internal/embed/pipeline.go`).

Today's chunking (`splitContent` in `internal/sync/ingest.go`) only ever sees README text and the shallow file-tree listing. It splits **purely by character count** at a budget computed by ADR-004:

```
maxChars = (maxTokens * 0.90) * 3
```

This is fine for prose but naive for code: a hard character cut lands mid-line, mid-identifier, or mid-function, producing chunks that embed poorly and read badly when cited. Code also has properties prose doesn't — it's line-structured, files vary wildly in size (a 5-line config vs a 5,000-line generated file), and a chunk's **file path** is essential context for a useful citation.

**Constraints:**
- Must respect each provider's token budget (Gemini ~2K, OpenAI ~8K, Voyage ~16K) — reuse ADR-004's formula, don't reinvent it.
- Must store the originating **file path** and an ordinal per chunk (PRD §4.4 schema change: `file_path`, `chunk_index`).
- Embedding batch size and the `chunk_embeddings` vec0 table are chunk-type-agnostic (keyed by `chunk_id`) — chunking changes must not require pipeline changes downstream.
- v0.3.0 ships on a deadline; perfect semantic chunking is not required to deliver value.

**Assumptions:**
- A chunk that respects line boundaries and stays within budget is "good enough" to materially improve RAG quality over today's prose-only corpus.
- Citations are far more useful when they carry `file_path` (e.g. `internal/sync/ingest.go`) than an opaque chunk id.
- Language-aware (AST) chunking is higher quality but high cost; it can be added later without re-architecting if we keep the chunk record shape stable.

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Embedding Quality | High | Do chunks form coherent, embeddable units? |
| Implementation Cost | High | v0.3.0 has a lot of surface area already |
| Token-Limit Safety | High | Must never exceed provider limits (reuse ADR-004) |
| Citation Usefulness | Medium | Path + boundaries make answers actionable |
| Language Generality | Medium | Should work for any text file, not just a few languages |
| Future Extensibility | Medium | Leaves room for semantic chunking later |

---

## Options

### Option A: Size-bounded, line-aware chunking (reuse ADR-004 budget)

Walk files, skip binary/oversized/ignored files (ADR-008 + `.repogignore`), then split each text file into chunks that:
- stay within the ADR-004 `maxChars` budget,
- break on **line boundaries** (never mid-line), packing whole lines until the next line would exceed budget,
- carry `file_path` and a `chunk_index` ordinal,
- optionally include a small header line (e.g. `// file: <path>`) inside each chunk to anchor context.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Embedding Quality | ★★☆ | Coherent line-bounded units; not function-aware but far better than mid-line cuts |
| Implementation Cost | ★★★ | Small extension of existing `splitContent` + budget logic |
| Token-Limit Safety | ★★★ | Reuses ADR-004 formula directly |
| Citation Usefulness | ★★★ | `file_path` + ordinal enables real citations |
| Language Generality | ★★★ | Works for any UTF-8 text file regardless of language |
| Future Extensibility | ★★★ | Chunk record shape is stable; can swap in AST chunking later |

**Trade-offs:**
- ✅ Cheap to build — extends machinery we already have
- ✅ Language-agnostic; no per-language parsers to maintain
- ✅ Stable chunk schema means semantic chunking can be added later without migration
- ❌ Not structure-aware — a chunk can still split a function across boundaries
- ❌ Header/overlap heuristics are approximations, not real semantics

---

### Option B: AST / tree-sitter semantic chunking

Parse each file with a language grammar (tree-sitter) and chunk on semantic boundaries (functions, classes, blocks), falling back to size-splitting for oversized units.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Embedding Quality | ★★★ | Chunks align to real code units — best retrieval quality |
| Implementation Cost | ★☆☆ | Grammars per language, CGO/bindings, fallback logic, lots of edge cases |
| Token-Limit Safety | ★★☆ | Still needs size fallback for huge functions |
| Citation Usefulness | ★★★ | Can cite "function X in file Y" |
| Language Generality | ★★☆ | Only as good as the grammars bundled; unknown languages fall back anyway |
| Future Extensibility | ★★★ | The eventual ideal |

**Trade-offs:**
- ✅ Highest retrieval quality; semantically meaningful citations
- ❌ Large implementation + dependency cost (grammars, likely more CGO on an already-CGO-heavy, large binary)
- ❌ Per-language coverage gaps; non-covered files fall back to size-splitting anyway
- ❌ Disproportionate cost for v0.3.0's goals

---

### Option C: Keep pure character-count splitting (status quo `splitContent`)

Apply today's mid-content character splitting unchanged to code files.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Embedding Quality | ★☆☆ | Mid-line / mid-token cuts degrade embeddings and citations |
| Implementation Cost | ★★★ | Zero new chunking code |
| Token-Limit Safety | ★★★ | Already enforces ADR-004 budget |
| Citation Usefulness | ★☆☆ | Without path/boundary awareness, citations are weak |
| Language Generality | ★★★ | Works on any text |
| Future Extensibility | ★★☆ | Would still need the schema change for paths eventually |

**Trade-offs:**
- ✅ No new chunking work
- ❌ Worst quality where it matters most (code is the whole point of the feature)
- ❌ Still needs the `file_path` schema change anyway, so the "free" framing is misleading

---

## Decision

**Chosen: Option A — size-bounded, line-aware chunking**, reusing the ADR-004 token→char budget and adding line-boundary packing plus `file_path` / `chunk_index` metadata.

Rationale:
1. It captures **most of the quality win** (coherent, in-budget, path-anchored chunks with real citations) at a **fraction of Option B's cost**.
2. It **reuses ADR-004** rather than introducing a parallel sizing scheme, keeping token-limit safety consistent across prose and code.
3. It is **language-agnostic**, so it works on the long tail of file types without per-language parser maintenance.
4. It **keeps the chunk record shape stable**, so Option B (AST/tree-sitter) can be layered in later as a fast-follow without a second migration — this ADR explicitly defers, not forecloses, semantic chunking.

---

## Implications

**Positives:**
- Minimal new code; builds on `splitContent` and ADR-004.
- Real, path-bearing citations flow through search/ask/summarize (PRD §4.4).
- No new heavy dependency on an already-large CGO binary.
- Works uniformly across languages and arbitrary text files.

**Negatives / Trade-offs:**
- Chunks are not function/class-aware; a unit of code can straddle a boundary, occasionally hurting retrieval precision.
- Per-file overhead (header lines, ordinals) slightly inflates chunk count and storage.
- Code corpora are much larger than prose — embedding time and DB size grow accordingly (document expected growth; mitigated by opt-in deep sync, size caps, and `.repogignore`).

**Watch out for:**
- **Very large / generated files** — enforce a size cap before reading (PRD Open Question #3) so a single 5 MB minified file doesn't dominate the corpus.
- **Binary/non-UTF-8 files** — detect and skip before chunking (ADR-008 walk/filter stage).
- **Embedding cost blowup** on multi-repo deep sync — keep deep sync opt-in and honor ignore rules.
- If retrieval precision proves insufficient, revisit Option B (tree-sitter) — the stable chunk schema makes that an additive change.

> Reference this ADR from relevant code: `// See ADR-009 for why code is chunked on line boundaries within the ADR-004 budget`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Developer (hackastak) | Pending — chunking approach + size caps flagged as PRD Open Questions #3/#4 | Confirm default max file size and ignore defaults |
| Claude Code | Analyzed `splitContent` + ADR-004 budget; weighed semantic vs size-based chunking against v0.3.0 scope | Recommended line-aware size chunking now, AST chunking deferred as a fast-follow |

---

## References

- Builds on: [ADR-004](./ADR-004-dynamic-chunking-based-on-model-token-limits.md) (token→char budget)
- Related: [ADR-008](./ADR-008-shell-to-system-git-for-deep-sync-cloning.md) (where files come from), [ADR-003](./ADR-003-clear-on-change-strategy-for-embedding-migrations.md) (re-embed semantics)
- PRD: `~/Developer/My_Notes/1. Projects/RepoG/PRD_V0.3.0.md` (§4.4 schema, Open Questions #3/#4)
- Code: `internal/sync/ingest.go` (`splitContent`, `CalculateMaxCharsFromTokens`), `internal/embed/pipeline.go`, `internal/db/schema.go` (chunks table + CHECK constraint + `UNIQUE(repo_id, chunk_type)`)
