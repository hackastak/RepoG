# ADR-001 Use SQLite with sqlite-vec for Vector Storage

---

## Status

`Decided`

---

## Context

RepoG needs to store and query vector embeddings for semantic search across repository code. The system must support:
- Storing embeddings (768-3072 dimensions depending on provider)
- Fast similarity search (k-nearest neighbors)
- Co-locating metadata with vectors (repo info, chunks, timestamps)
- Local-first operation without external dependencies
- Simple deployment and zero configuration

**Constraints:**
- CLI tool targeting individual developers and small teams
- Must run on macOS, Linux, and Windows
- Users should not need to manage separate database servers
- Embedding data is sensitive (user's private code)
- Must support air-gapped/offline development environments

**Assumptions:**
- Dataset size: 10-500 repositories per user
- Vector count: 100-10,000 embeddings per typical use case
- Query latency: <100ms acceptable for interactive CLI
- No need for distributed scaling or multi-user concurrency

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Operational Complexity | High | Zero-config, no server management |
| Developer Experience | High | Easy setup, no dependencies |
| Portability | High | Single binary distribution, cross-platform |
| Local-first / Privacy | High | No cloud dependencies, data stays local |
| Performance / Latency | Medium | Fast enough for CLI use (~100ms queries) |
| Cost | Medium | Free for users |
| Vector Search Quality | Medium | Good-enough similarity search |
| Scalability | Low | Only need to handle 100-10K vectors |

---

## Options

### Option A: SQLite + sqlite-vec

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Operational Complexity | ★★★ | Zero-config, embedded database |
| Developer Experience | ★★★ | Single file, no setup, works offline |
| Portability | ★★★ | Cross-platform, single binary via CGO |
| Local-first / Privacy | ★★★ | Fully local, no network required |
| Performance / Latency | ★★☆ | ~20-50ms for 10K vectors, acceptable for CLI |
| Cost | ★★★ | Free, open source |
| Vector Search Quality | ★★☆ | Brute-force cosine similarity, good for small datasets |
| Scalability | ★☆☆ | Linear scan, degrades beyond ~100K vectors |

**Trade-offs:**
- ✅ Single `.db` file, trivial to backup/restore
- ✅ No external dependencies, works offline
- ✅ Familiar SQL + vector operations
- ✅ Co-locates metadata and vectors in one database
- ❌ Not optimized for large-scale vector search (no HNSW)
- ❌ Requires CGO (C compiler needed for builds)

---

### Option B: Dedicated Vector Database (Pinecone, Weaviate, Qdrant)

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Operational Complexity | ★☆☆ | Requires cloud account or local server |
| Developer Experience | ★☆☆ | API keys, network config, separate service |
| Portability | ★☆☆ | Requires internet or Docker |
| Local-first / Privacy | ★☆☆ | Cloud-hosted (Pinecone) or self-hosted server |
| Performance / Latency | ★★★ | <10ms queries, HNSW/IVF optimized |
| Cost | ★☆☆ | Pinecone costs money, self-hosted needs resources |
| Vector Search Quality | ★★★ | State-of-the-art ANN algorithms |
| Scalability | ★★★ | Designed for millions of vectors |

**Trade-offs:**
- ✅ Best-in-class vector search performance
- ✅ Optimized for large datasets (millions of vectors)
- ❌ Requires managing separate service (cloud or Docker)
- ❌ Not local-first (cloud) or high operational burden (self-hosted)
- ❌ Adds network dependency and latency
- ❌ Privacy concerns if cloud-hosted

---

### Option C: PostgreSQL + pgvector

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Operational Complexity | ★☆☆ | Requires Postgres server installation |
| Developer Experience | ★★☆ | Familiar SQL, but requires Postgres setup |
| Portability | ★☆☆ | Users must install/configure Postgres |
| Local-first / Privacy | ★★☆ | Can run locally, but needs server process |
| Performance / Latency | ★★★ | HNSW index support, fast queries |
| Cost | ★★★ | Free, open source |
| Vector Search Quality | ★★★ | HNSW index, production-quality |
| Scalability | ★★★ | Handles large datasets well |

**Trade-offs:**
- ✅ Production-grade vector search with HNSW
- ✅ Rich SQL ecosystem and tooling
- ✅ Can scale to large datasets
- ❌ Requires Postgres server (non-trivial setup)
- ❌ Not embeddable, adds deployment complexity
- ❌ Overkill for CLI tool with small datasets

---

## Decision

We chose **Option A (SQLite + sqlite-vec)** because it scores highest against our top priorities — operational complexity, developer experience, portability, and local-first operation — and its limited scalability is acceptable given:

1. **Zero-config deployment**: Single binary with embedded database, no installation or setup required
2. **Local-first and private**: All data stays on the user's machine, works offline
3. **Perfect for CLI use case**: Target dataset size (100-10K vectors) fits sqlite-vec's performance profile
4. **Simple backup/restore**: Single `.db` file can be easily copied, versioned, or synced
5. **Cross-platform**: Works identically on macOS, Linux, Windows via CGO
6. **No cost or external dependencies**: Free, no API keys, no cloud accounts

The performance trade-off (linear scan vs. HNSW) is acceptable because:
- Typical use case is <10,000 vectors
- Query latency of 20-50ms is imperceptible in a CLI
- Simplicity and zero-config outweigh marginal performance gains

---

## Implications

**Positives:**
- Users can start using RepoG with `repog init` and no other setup
- Database is a single `.db` file at `~/.repog/repog.db` (easy to backup)
- Works completely offline and air-gapped
- No ongoing costs or cloud dependencies
- Cross-platform support via CGO compilation
- Familiar SQL for queries and debugging

**Negatives / Trade-offs:**
- CGO dependency requires C compiler (GCC/Clang) for building from source
- Vector search uses brute-force cosine similarity (no HNSW index)
- Performance degrades linearly beyond ~100K vectors
- Cannot distribute queries across multiple machines
- sqlite-vec is less mature than pgvector or dedicated vector DBs

**Watch out for:**
- If users report slow searches (>500ms), investigate vector count and consider sampling
- If dataset grows beyond 100K vectors, may need migration path to pgvector or dedicated DB
- CGO build complexity may require pre-compiled binaries for distribution
- WAL mode required for concurrent reads during embedding (already implemented)

> Reference this ADR from relevant code: `// See ADR-001 for why we use SQLite + sqlite-vec`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Target users (developers) | Need zero-config, local-first tool; don't want to manage servers | Strongly favored embedded database approach |
| sqlite-vec maintainer (asg017) | Library designed for exactly this use case (small-medium datasets) | Confirmed sqlite-vec is appropriate choice |

---

## References

- Related ADRs: None (foundational decision)
- Technical docs: [sqlite-vec documentation](https://github.com/asg017/sqlite-vec)
- Supporting code: `internal/db/db.go` - Database initialization and schema
- Supporting code: `internal/search/query.go` - Vector similarity queries
- Alternative: [pgvector](https://github.com/pgvector/pgvector) - Postgres extension
- Alternative: [Chroma](https://www.trychroma.com/) - Dedicated vector database
