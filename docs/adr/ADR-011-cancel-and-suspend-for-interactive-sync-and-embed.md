# ADR-011 Cancel and Suspend for Interactive Sync and Embed Runs

---

## Status

`Decided` (2026-08-06). Builds directly on the cancel-on-quit work landed on `chore/post-rewrite-cleanup` (the four TUI streaming views now tear down their producer goroutines when the app quits). This ADR records the design for two *deliberate*, user-driven controls layered on that plumbing: **cancel** (stop and abandon a run) and **suspend/resume** (halt a run and pick it back up). TUI-only; the CLI keeps Ctrl-C cancellation.

---

## Context

The Sync/Embed view runs two long pipelines — `sync.IngestRepos` and `embed.RunEmbedPipeline` — that can take minutes over hundreds of repositories. Until now the only way to stop one from the TUI was to quit the whole app (`q`/`ctrl+c`), which cancels the run as a side effect of exiting. There is no way to **stop a run and keep using the TUI**, and no way to **pause a run and resume it later**.

The developer asked for two distinct operations, not one:

- **Cancel** — kill the running operation now and return to idle. "I don't want this run anymore."
- **Suspend / Resume** — halt the run but keep a resume point. "Pause this; I'll continue in a moment."

The word "pause" is intentionally split from "cancel" because they mean different things to the user, even where they share machinery underneath.

### What the pipelines already guarantee

Three properties (all verified in code as of this branch) determine which implementations are even on the table:

- **Sync is resumable by construction.** Each repository is its own transaction; `pushed_at_hash` skips unchanged repos on a re-run; `sync_state` is written *per repo, inside that repo's transaction* (not as a run-level "completed" flag), so a stopped run's in-flight repo rolls back cleanly. The ingest loop now breaks on `ctx.Err()` between repositories.
- **Embed is resumable by construction.** Chunk embeddings persist to `chunk_embeddings` as each succeeds; a repo's `embedded_hash` is advanced **only** when every one of its chunks succeeds (`counts.processed == counts.total`); the batch loop breaks on `ctx.Err()` between batches; a re-run re-embeds only chunks that lack a `chunk_embeddings` row.
- **The teardown plumbing exists.** `syncView` owns a `streamCtx`/`cancel`, threads the context into both pipelines, and races `ctx.Done()` on every event-channel read, so cancelling the context unwinds both the consumer command and the producer goroutine.

The consequence: the database is already a durable, idempotent record of progress. Re-issuing a stopped run redoes only the remaining work, because completed units are skipped on their hashes / embedding rows. This is an existing property of incremental sync/embed — a *manual* re-run benefits from it too — not an automatic resume; this ADR only adds an in-session convenience on top of it.

### Constraints

- **Must not regress the leak fix.** Whatever cancel/suspend do, they must not strand a producer goroutine or hold an HTTP request open.
- **Must not corrupt resume state.** A stopped-mid-embed repo must never be marked fully embedded; a stopped-mid-sync repo must not be half-persisted.
- **TUI-only.** Cancel/suspend are interactive; the CLI's equivalent of "resume" is simply re-running the command, which already continues from DB state.
- **Thin presentation layer (ADR-010).** No business logic moves into the TUI; `internal/sync` and `internal/embed` stay authoritative.

### Assumption

Losing at most one unit of in-flight work (the repository or batch being processed at the moment of stop) and redoing it on resume is acceptable. For embed that unit can carry real API cost, but it is bounded to a single batch.

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Correctness / resume-state safety | High | Never mark a partially-embedded repo done; never half-persist a repo |
| Implementation cost / complexity | High | Solo maintainer; prefer reusing the just-landed cancel plumbing |
| Resource behavior while halted | High | A "paused" run must not hold HTTP connections open into provider/client timeouts |
| Efficient re-run after a stop | Medium | Does a later run skip work already done rather than redoing it? |
| Resume fidelity | Medium | Does resume continue the *exact* in-flight run, or restart and skip? |

---

## Options

### Option A: Suspend via context-cancel + resume from persisted state

Both cancel and suspend cancel `streamCtx` — identical teardown. They differ only in **UI state**: cancel returns the view to idle and drops the resume point; suspend enters a `paused` state that remembers what to resume (`pausedPhase`, `pausedChain`). **Resume re-issues the same pipeline command**, which skips already-done work via `pushed_at_hash` / `chunk_embeddings` and processes only the remainder.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Correctness / resume-state safety | ★★★ | Reuses the transactional + `embedded_hash` guarantees already tested |
| Implementation cost / complexity | ★★★ | ~1 file (`internal/tui/sync.go`); no pipeline changes |
| Resource behavior while halted | ★★★ | Nothing held — goroutines exit, connections close, on stop |
| Efficient re-run after a stop | ★★★ | The DB's incremental skip means a later run redoes no completed work |
| Resume fidelity | ★★☆ | Resume restarts the run and skips done work; only the in-flight unit is redone |

**Trade-offs:**
- ✅ Tiny change resting entirely on plumbing that just landed
- ✅ No held connections; a suspended run consumes zero resources
- ✅ A later run redoes no completed work — the DB's incremental skip handles it (independent of this feature)
- ❌ Not a true freeze — the in-flight repo/batch is discarded and redone on resume (bounded to one unit)
- ❌ Continuous counters need a small amount of view state (a baseline snapshot, plus a `seenRepos` set for sync)

### Option B: True in-memory suspension

Keep the run's goroutine alive but parked on a condition variable / paused channel, holding its place in the batch loop and its HTTP client, so resume continues the *exact* in-flight run with no rework.

| Criterion | Score | Notes |
|---|---|---|
| Correctness / resume-state safety | ★★☆ | New concurrency surface (park/unpark, cancel-while-parked) to get wrong |
| Implementation cost / complexity | ★☆☆ | Requires pause/resume primitives inside `internal/sync` + `internal/embed` |
| Resource behavior while halted | ★☆☆ | Holds HTTP connections and an open SQLite write open indefinitely |
| Efficient re-run after a stop | ★★☆ | A re-run still skips done work via the DB, but the parked run itself dies with the process |
| Resume fidelity | ★★★ | Continues the identical run; no unit redone |

**Trade-offs:**
- ✅ No rework; resume is byte-for-byte continuation
- ❌ A paused run holds a provider HTTP request and an open DB transaction — exactly the resource leak the last change removed, now made deliberate; providers will time the request out anyway
- ❌ Pushes pause/resume state and new concurrency into the pipelines, violating the thin-presentation-layer intent (ADR-010)
- ❌ Buys fidelity the DB already makes unnecessary

### Option C: Cancel-only (no suspend)

Add a dedicated "stop this run" key but no resume affordance. Re-running `s`/`e` later continues from DB state anyway.

| Criterion | Score | Notes |
|---|---|---|
| Correctness / resume-state safety | ★★★ | Same teardown as today's quit path |
| Implementation cost / complexity | ★★★ | Smallest — one key, one transition to idle |
| Resource behavior while halted | ★★★ | Nothing held |
| Efficient re-run after a stop | ★★★ | Via re-run; the DB skips done work |
| Resume fidelity | ★☆☆ | No in-session resume; the user must re-trigger and mentally track that it continues |

**Trade-offs:**
- ✅ Cheapest; covers the "I want to stop" need
- ❌ Ignores the explicit ask for a resume point; the user loses the run's log/progress context and has to re-trigger by hand

---

## Decision

**Chosen: Option A — suspend via context-cancel with resume from persisted DB state; cancel and suspend as two UI states over the same teardown.**

Rationale:
1. The pipelines are **already idempotent and resumable from the DB**, so Option B's fidelity buys nothing that matters while reintroducing held HTTP connections and an open SQLite write — the precise resource leak the preceding change eliminated. Deliberately re-creating it is the wrong direction.
2. Option A is a **~1-file change** on `internal/tui/sync.go` that reuses the `streamCtx`/`cancel`/`ctx.Done()` plumbing verbatim; `internal/sync` and `internal/embed` need no changes, honoring the ADR-010 thin-layer intent.
3. The database is already the source of truth for progress, so nothing about a run needs to be held in memory or in an open connection between stop and continue. A later run — resumed or fresh — redoes no completed work via the existing incremental skip. (Suspend itself is deliberately *not* persisted: quit is a hard cancel, so there is no on-disk paused run to restore.)
4. Option C alone under-serves the explicit request for a resume point.

**The two operations, concretely:**

| Op | Key | Effect | State after |
|---|---|---|---|
| Suspend | `p` (while running) | Cancel `streamCtx`; capture `pausedPhase`/`pausedChain` and a counter baseline; `gen++` to discard in-flight events | `paused` — `r` resumes |
| Resume | `r` (while paused) | Re-issue `beginSync(pausedChain)` or `beginEmbed(true)` without zeroing counters (render `baseline + live`) | `running` |
| Cancel | `c` (while running) | Cancel `streamCtx`; drop any resume point and counter baseline; `gen++` | `idle` |
| Quit | `q` / `ctrl+c` | Cancel every stream outright (existing `cancelStreams` sweep); nothing persisted | app exits |

**Quit is a hard cancel, not a suspend.** Suspend/resume and the carried counts are an *in-session, in-memory* affordance only. Nothing about a paused run is written to disk, so quitting drops it entirely: the next launch starts fresh, with fresh counters and no "resume the previous run" prompt.

**Two different "resume-ish" behaviors, only one of which this feature adds.** Do not conflate them:
- **Session resume** (this feature): the `p`/`r` continue point plus carried counts. In-memory; gone on quit.
- **DB incremental skip** (pre-existing, independent of this feature): because embeddings and synced repos persist as they complete, *any* later run — a resume, a fresh manual `e`/`s`, or next week's sync — skips work already done (`chunk_embeddings` existence, `pushed_at_hash`/`embedded_hash` match). This is not "resuming a suspended run"; it is incremental sync/embed working as designed, and it is why `embedded_hash`/`pushed_at_hash` exist.

**What cancel/suspend/quit do and don't touch on disk, recorded so no one mistakes it later:** all three stop the pipeline identically and **none rolls back work already committed.** Embeddings and synced repos already written stay — discarding them would throw away API spend already paid. The in-flight *unit* at the moment of stop (the current repo's transaction, the current batch) is discarded and redone on the next run. So the difference between cancel and suspend is purely the in-session resume affordance, not what lands on disk.

**Carried counters.** On suspend, the view snapshots its counters as a baseline; resume renders `baseline + live-run`. For **embed** this is exact with no extra bookkeeping — the resumed pipeline fetches only un-embedded chunks (`internal/embed/pipeline.go` `AND NOT EXISTS (… chunk_embeddings …)`), so finished chunks are never re-counted. For **sync** the resumed run re-emits already-synced repos as `skip` events, which would migrate the `synced` tally into `skipped`; a `seenRepos` set in view state dedupes per-repo events so each repo is counted once. Counters are in-memory, so a quit drops them — by design, per the hard-cancel rule above.

Keys `p`/`r` were chosen for suspend/resume (mnemonic, free in-view and globally, k9s/lazygit idiom per ADR-010); `c` for cancel.

---

## Implications

**Positives:**
- Suspend/cancel are a small, contained addition over existing plumbing; no pipeline or schema changes.
- A halted run holds no resources — goroutines exit and connections close on stop.
- Quit is a clean hard cancel: nothing is persisted, so there is no half-alive "paused" state to reconcile on the next launch. A later manual run still avoids redoing completed work through the pre-existing incremental skip, independent of this feature.
- The model generalizes to V0.4.0 deep sync, which will add more per-repo work behind the same event channels.

**Negatives / Trade-offs:**
- **Suspend is not a true freeze.** The repo or batch in flight at the moment of `p` is discarded (its transaction rolls back / its batch stops) and redone on resume. Wasted work is bounded to one unit, but for embed that unit costs provider tokens.
- **Counters are carried across suspend, in memory only.** Resume renders `baseline + live` so the numbers continue rather than resetting; sync needs a `seenRepos` set to keep re-emitted skips from double-counting. This state is deliberately not persisted, so a quit drops it and the next launch shows fresh counters — consistent with quit being a hard cancel.
- **Cancel does not purge partial progress** — by design. If a user reads "cancel" as "undo everything this run wrote," that expectation is not met (and meeting it would waste paid-for embeddings). Documented in the help text.
- **Reconfig discards a paused run.** The Settings flow rebuilds every view (`buildViews`), constructing a fresh `syncView` and dropping `paused` state. The DB still allows a manual re-run to continue.

**Watch out for:**
- **`embedded_hash` invariant** — a suspend mid-embed must never advance it; the `processed == total` gate already guarantees this and the partial-batch test pins it. Extend that test with a suspend-then-resume case.
- **`gen++` on suspend and cancel** — required so an event already queued from the stopped run is discarded by the existing generation guard rather than mutating the new state.
- **Spinner ticker on resume** — `beginEmbed(true)` must issue its own `spinner.Tick`; the existing `issueTick` flag covers this, but verify resume neither double-ticks nor drops the spinner.
- **`sync_state`** — confirmed per-repo and transactional; there is no run-level completion flag to clear on a stopped run.

> Reference this ADR from relevant code: `// See ADR-011 for cancel vs suspend semantics`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Developer (hackastak) | Directed that the feature be two operations — cancel (kill the run) and suspend (pause to resume later) — and chose an in-repo ADR plus `p`/`r` keys | Split the design into distinct cancel and suspend controls; `c` added for cancel |
| Claude Code | Verified the sync/embed resumability guarantees (per-repo transactions, `sync_state` scope, `embedded_hash` gate) and that Option B would re-introduce the just-fixed held-connection leak | Recommended Option A: resume from persisted state over true in-memory suspension |

---

## References

- Builds on: TUI stream cancel-on-quit for all four streaming views (`chore/post-rewrite-cleanup`; `internal/tui/sync.go` `streamCtx`/`cancel`, `internal/sync/ingest.go` `ctx.Err()` loop break)
- Related: [ADR-010](./ADR-010-use-bubbletea-for-the-tui.md) (TUI as a thin presentation layer over event channels; "long-running ops must stay cancellable from the UI")
- Code: `internal/tui/sync.go` (`beginSync`/`beginEmbed`, `finishSync`/`finishEmbed`, wait commands); `internal/embed/pipeline.go` (`embedded_hash` gate, batch `ctx.Err()` check); `internal/sync/ingest.go` (per-repo transaction, `sync_state`, `ctx.Err()` break)
- Backlog: pause/resume for sync + embed (RepoG Backlog, Nice-to-Have / UX)
