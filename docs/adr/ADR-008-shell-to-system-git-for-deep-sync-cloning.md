# ADR-008 Shell to System Git for Deep-Sync Cloning

---

## Status

`Decided` (2026-06-10) — **implementation deferred to V0.5.0**. Originally split out of V0.3.0 to V0.4.0 (2026-06-11), then rescheduled again once V0.4.0 landed as provider-hardening rather than deep sync. The clone-strategy decision below stands; full code base syncing was split out of V0.3.0 (now TUI-only, see [ADR-010](./ADR-010-use-bubbletea-for-the-tui.md)) so the TUI ships on its own predictable scope and deep sync's embedding-cost/quality trade-offs get a dedicated release. Resolves [PRD_V0.3.0 Open Question #1](../../../My_Notes/1.%20Projects/RepoG/PRD_V0.3.0.md).

---

## Context

V0.3.0 introduces **full code base syncing**: instead of ingesting only metadata, README, and a shallow file-tree *listing* via the GitHub REST API (`internal/sync/ingest.go`, `internal/github/repos.go`), RepoG will fetch the **actual file contents** of selected repos, walk the tree from disk, and chunk real code.

To get file contents onto disk we need a fetch mechanism. The current codebase has **no git dependency of any kind** — no `go-git`, no `git2go`, no `exec.Command("git")`. This is a greenfield decision.

**Constraints:**
- CGO is already required (sqlite-vec, go-sqlite3) and the binary is ~15–20 MB (ADR-006). We don't want to bloat it further without reason.
- Distribution is **macOS-only Homebrew today**; Linux Homebrew is a backlog goal blocked by the sqlite-vec + musl cross-compile pain (see [[Backlog]] / [[WTF]]). Any new build-time native dependency makes that harder.
- Must support **private repos** — the fetch must authenticate with the stored GitHub PAT (keyring) without ever writing the PAT to disk or logs.
- Fetches must be **shallow/fast** — we only need a snapshot of the default branch, not history.
- Temp artifacts must be **reliably cleaned up** (success, error, and Ctrl-C).

**Assumptions:**
- We only need a point-in-time snapshot of one branch; we do not need git history, blame, or multiple refs for v0.3.0.
- Re-sync is re-fetch-from-scratch for v0.3.0 (incremental `git pull` is a non-goal — see PRD N2).
- Developers running a CLI dev tool installed via Homebrew almost certainly have `git` on their PATH (Xcode Command Line Tools ship it; Homebrew itself depends on it).

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Binary Size / Build Simplicity | High | Avoid worsening the ~15–20 MB binary and the musl cross-compile situation |
| Runtime Dependencies | Medium | A dependency on a `git` binary on PATH is a usability/onboarding cost |
| Auth for Private Repos | High | Must inject PAT securely, no disk/log leakage |
| Fetch Speed | Medium | Shallow snapshot should be fast |
| Robustness / Maintenance | Medium | Edge cases: LFS, submodules, large repos, network failures |
| Cross-platform | Medium | Must work on macOS now, Linux later |

---

## Options

### Option A: Shell out to system `git` (shallow clone)

Run `git clone --depth=1 --single-branch --branch <default>` into an `os.MkdirTemp` dir, with the PAT injected via an in-memory credential mechanism (e.g. `GIT_ASKPASS`/credential helper or an `https://x-access-token:<PAT>@github.com/...` URL kept only in memory).

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Binary Size / Build Simplicity | ★★★ | Zero added Go deps; no impact on binary size or musl cross-compile |
| Runtime Dependencies | ★☆☆ | Requires `git` on PATH at runtime |
| Auth for Private Repos | ★★★ | Standard, battle-tested git credential paths |
| Fetch Speed | ★★★ | Native git, full protocol optimizations, shallow clone |
| Robustness / Maintenance | ★★★ | git handles LFS, submodules, redirects, retries natively |
| Cross-platform | ★★★ | git behaves consistently; just needs to be installed |

**Trade-offs:**
- ✅ No binary bloat, no new native build dependency — keeps the Linux cross-compile story unchanged
- ✅ Most robust fetch (LFS/submodule/edge-case handling is git's problem, not ours)
- ✅ Fastest clone via native protocol
- ❌ Runtime dependency on `git` being installed and on PATH
- ❌ Must guard the PAT carefully so it never lands in process args visible via `ps`, logs, or temp credential files
- ❌ Shelling out means parsing/handling `git` exit codes and stderr for good error messages

---

### Option B: Pure-Go clone via `go-git`

Use `github.com/go-git/go-git/v5` to clone in-process with `Depth: 1`.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Binary Size / Build Simplicity | ★☆☆ | Large pure-Go dependency tree; meaningfully grows the binary |
| Runtime Dependencies | ★★★ | No external binary needed — self-contained |
| Auth for Private Repos | ★★★ | `http.BasicAuth{Username: "x-access-token", Password: PAT}` in memory |
| Fetch Speed | ★★☆ | Slower than native git on large repos; shallow support has historical rough edges |
| Robustness / Maintenance | ★★☆ | No native LFS/submodule parity; some protocol edge cases differ from canonical git |
| Cross-platform | ★★★ | Pure Go — same everywhere, no PATH assumptions |

**Trade-offs:**
- ✅ Self-contained: no `git` on PATH required — best onboarding story
- ✅ Clean in-process auth with the PAT held only in memory
- ❌ Adds a heavy dependency to a binary that's already large; works against ADR-006 sizing and the musl goal
- ❌ Weaker parity with canonical git for LFS, submodules, and some shallow-clone scenarios
- ❌ Generally slower on large repositories

---

### Option C: Download tarball via GitHub Archive API

Fetch `GET /repos/{owner}/{repo}/tarball/{ref}` over the existing authenticated HTTP client (`internal/github`), stream to a temp dir, and extract.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Binary Size / Build Simplicity | ★★★ | Uses stdlib `archive/tar` + `compress/gzip`; no new deps |
| Runtime Dependencies | ★★★ | None — reuses existing HTTP client + keyring auth |
| Auth for Private Repos | ★★★ | Same Bearer-token client already used for the REST API |
| Fetch Speed | ★★★ | Single compressed download, no git protocol negotiation |
| Robustness / Maintenance | ★★☆ | No `.git` metadata; counts against REST rate limit; redirects to a signed URL to follow |
| Cross-platform | ★★★ | Pure stdlib extraction |

**Trade-offs:**
- ✅ No new dependency at all — reuses the existing authenticated GitHub client and keyring path
- ✅ No `git` on PATH required; smallest blast radius
- ✅ Naturally gives a clean snapshot with no `.git/` to filter out
- ❌ Consumes GitHub REST API rate limit (deep-sync of many repos competes with metadata sync); the existing token-rotation work (v0.2.3) helps but doesn't eliminate this
- ❌ No `.git` metadata means no future path to incremental `git pull` without re-architecting
- ❌ Must handle the archive redirect, gzip+tar streaming, and path-traversal safety on extraction ourselves

---

## Decision

**Chosen: Option A (shell out to system `git`) — no fallback.**

Rationale:
1. It adds **zero build-time weight** to an already-large CGO binary and leaves the unresolved Linux/musl cross-compile situation exactly as-is — important given Linux Homebrew is still a backlog goal.
2. It is the **most robust** fetch (LFS, submodules, redirects, retries are git's responsibility), which matters because deep sync runs against arbitrary user repos of unknown shape.
3. The main downside — a runtime `git` dependency — is a **non-issue for our audience**: RepoG is a developer tool for working with GitHub repos, so anyone running it will already have the `git` CLI installed. A user without `git` is not a real use case worth designing for.
4. We deliberately **drop the Option C tarball fallback**. Maintaining a second fetch path to serve a near-empty population (developers without `git`) is not worth the added surface area. If `git` is genuinely absent, a single clear startup error on `--deep` is the correct behavior, not a silent fallback.

---

## Implications

**Positives:**
- No change to binary size or the musl cross-compile blocker.
- Strong robustness on real-world repos with minimal code we have to maintain.
- Private-repo auth uses well-trodden git credential paths.

**Negatives / Trade-offs:**
- Introduces a runtime prerequisite (`git` on PATH); should be declared in the Homebrew formula and checked at the start of a `--deep` sync with an actionable error. (Low real-world impact — our users have `git`.)
- We must take care that the PAT is never exposed via process arguments (`ps`), credential files on disk, or event/log output — use an in-memory `GIT_ASKPASS` or credential-helper approach, not a token-in-URL that could be logged.

**Watch out for:**
- **PAT leakage** — the single most important security property. Audit every place the clone URL or credentials could be printed (events, verbose logs, error messages).
- **Temp dir cleanup** on Ctrl-C / SIGINT — register signal handling so partial clones are removed (PRD success criterion).
- **Missing `git`** — detect at the start of a `--deep` sync and fail with a clear, actionable message (e.g. "deep sync requires the `git` CLI on your PATH"); do not fall back silently.
- If incremental re-sync (`git pull`) becomes a priority later, shelling to `git` keeps `.git/` available to enable it.

> Reference this ADR from relevant code: `// See ADR-008 for why deep sync shells out to system git`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Developer (hackastak) | Decided Option A, no fallback — RepoG users are developers working with GitHub repos and will already have the `git` CLI installed, so the runtime prerequisite is a non-issue and Option C is unnecessary surface area | Final decision: exec system `git`, fail clearly if absent |
| Claude Code | Mapped the absence of any git dependency, weighed binary-size vs robustness vs runtime-dep | Originally recommended exec-git with a tarball fallback; fallback dropped per developer decision |

---

## References

- Related: [ADR-001](./ADR-001-use-sqlite-with-sqlite-vec-for-vector-storage.md) (local-first ethos), [ADR-006](./ADR-006-cli-architecture-with-cobra.md) (binary-size sensitivity)
- PRD: `~/Developer/My_Notes/1. Projects/RepoG/PRD_V0.3.0.md` (§4.1, §5, Open Question #1)
- Code: `internal/sync/ingest.go` (current API-only sync), `internal/github/repos.go` (existing authenticated client)
- Context: Linux/musl cross-compile constraint per project backlog and [[WTF]] note
