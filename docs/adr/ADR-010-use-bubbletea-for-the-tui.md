# ADR-010 Use Bubbletea for the Interactive TUI

---

## Status

`Decided` (2026-06-12) — the **sole V0.3.0 deliverable**: full code base syncing ([ADR-008](./ADR-008-shell-to-system-git-for-deep-sync-cloning.md) / [ADR-009](./ADR-009-size-bounded-line-aware-chunking-for-code.md)) was deferred to V0.4.0 (2026-06-11), so V0.3.0 ships the TUI on its own. Confirmed by the developer: Bubbletea is the chosen framework; the added binary size and new dependencies are a calculated cost, far outweighed by the improved user experience. See [PRD_V0.3.0 §4.3, Open Question #5](../../../My_Notes/1.%20Projects/RepoG/PRD_V0.3.0.md).

---

## Context

V0.3.0 adds an interactive **TUI**: the bare `repog` command (no subcommand) should open a visual interface for browsing indexed repos, searching, asking questions, and driving sync/embed — instead of requiring users to memorize 9 subcommands and their flags.

### Resolved UX decisions (2026-06-12)

Confirmed with the developer before implementation began:

1. **Navigation — tabbed dashboard.** A persistent top tab bar; switch views with `1`–`5` / `tab` (k9s / lazygit style), so every primary view is one keystroke away. Chosen over a menu drill-down or a list+detail sidebar.
2. **Command coverage — all nine CLI commands**, not just the five MVP views. The tab bar hosts the five primary views (Repos · Search · Ask · Sync/Embed · Status); the remaining commands fold in contextually rather than each getting a tab:
   - `recommend` and `summarize` → **per-repo actions** invoked from the Repos view (operate on the selected repo).
   - `init` / `reconfig` → a **Settings** screen (reachable by key, not a numbered tab); the first-run setup flow is the same screen in its empty state.
3. **First run — in-TUI setup flow.** When bare `repog` launches on a TTY but no config/credentials exist, the TUI detects this and walks the user through credential entry (textinput → keyring) rather than printing "run `repog init`". On a non-TTY it still degrades to CLI behavior.

The current CLI uses **non-TUI** terminal libraries: `AlecAivazis/survey/v2` (interactive prompts during `init`/`reconfig`), `briandowns/spinner` (progress spinners), and `fatih/color` (colored output). None of these is a full-screen application framework — there is **no Bubbletea, tview, tcell, or tview** in `go.mod` today. This is a greenfield UI-framework decision.

The TUI is a **presentation layer** over the existing internal packages (`sync`, `embed`, `search`, `ask`, `recommend`, `summarize`, `status`). Those packages already expose streaming **event channels** (e.g. `IngestRepos`, `RunEmbedPipeline` return `<-chan …Event`), which a reactive UI can consume directly for live progress.

**Constraints:**
- Must not regress the existing CLI — every subcommand keeps working unchanged; the TUI is additive.
- Must **degrade gracefully on non-TTY** (piped output, CI) — fall back to CLI behavior, never hang waiting for a TTY.
- Should integrate cleanly with the existing event-channel design for live sync/embed progress.
- Respect `NO_COLOR` and reasonable terminal-capability detection.
- Mindful of binary size (already ~15–20 MB per ADR-006) — but a TUI framework is the point, so some growth is acceptable.

**Assumptions:**
- An Elm-style (Model-Update-View) architecture maps well onto our event-channel streams (events become messages).
- The same maintainer (solo) will own this; framework ergonomics and ecosystem matter for velocity.

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Developer Velocity / Ergonomics | High | Solo maintainer; how fast can we build & iterate views? |
| Ecosystem / Components | High | Prebuilt table/list/viewport/spinner/textinput? |
| Fit with Event Channels | High | Live sync/embed progress streaming into the UI |
| Maturity / Community | Medium | Longevity, docs, examples |
| Binary Size Impact | Medium | Already-large binary |
| Styling / Polish | Medium | "Visually pleasing" is an explicit backlog goal |

---

## Options

### Option A: Bubbletea (+ Lipgloss + Bubbles)

`charmbracelet/bubbletea` (Elm architecture) with `lipgloss` (styling) and `bubbles` (prebuilt components: table, list, viewport, spinner, textinput, paginator).

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Velocity / Ergonomics | ★★★ | Elm Model-Update-View is simple and predictable; great docs/examples |
| Ecosystem / Components | ★★★ | `bubbles` gives table/list/viewport/textinput/spinner out of the box |
| Fit with Event Channels | ★★★ | Channel reads become `tea.Msg`s via `tea.Cmd` — natural fit for our event streams |
| Maturity / Community | ★★★ | De-facto standard for modern Go TUIs; large community, active |
| Binary Size Impact | ★★☆ | Adds the Charm stack; moderate growth, acceptable for the feature |
| Styling / Polish | ★★★ | Lipgloss makes "visually pleasing" (backlog goal) straightforward |

**Trade-offs:**
- ✅ Best ergonomics + richest component ecosystem for fast view-building
- ✅ Event-channel streams map cleanly to the message loop (live progress for free)
- ✅ Lipgloss directly serves the "visually pleasing UI" backlog goal
- ❌ Elm architecture is a paradigm shift if unfamiliar (one-time learning cost)
- ❌ Adds several dependencies and some binary weight

---

### Option B: tview (on tcell)

`rivo/tview` — higher-level widget toolkit (forms, tables, flex layouts) built on `gdamore/tcell`.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Velocity / Ergonomics | ★★☆ | Widget/callback model; quick for forms, more imperative state handling |
| Ecosystem / Components | ★★★ | Rich built-in widgets (Table, List, TextView, Flex, Pages) |
| Fit with Event Channels | ★★☆ | Must marshal channel events onto the UI thread via `QueueUpdateDraw` |
| Maturity / Community | ★★★ | Mature, stable, widely used |
| Binary Size Impact | ★★☆ | Comparable footprint |
| Styling / Polish | ★★☆ | Functional styling; less expressive than lipgloss |

**Trade-offs:**
- ✅ Fast to assemble conventional widget screens; very stable
- ✅ Strong built-in widget set
- ❌ Imperative callback/state model is harder to reason about as views grow
- ❌ Channel→UI integration is more manual (`QueueUpdateDraw`) than Bubbletea's message loop
- ❌ Less expressive styling for the "visually pleasing" goal

---

### Option C: No TUI framework — enhance CLI with survey + color

Skip a full-screen UI; make bare `repog` an interactive `survey`-driven menu that dispatches to existing commands.

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Velocity / Ergonomics | ★★★ | Reuses libraries already in the project |
| Ecosystem / Components | ★☆☆ | Prompts only — no tables, panes, or live dashboards |
| Fit with Event Channels | ★☆☆ | Can print streamed events but no rich live layout |
| Maturity / Community | ★★★ | Already a dependency |
| Binary Size Impact | ★★★ | No new dependency |
| Styling / Polish | ★☆☆ | Cannot deliver the "visually pleasing interface" goal |

**Trade-offs:**
- ✅ Zero new dependencies; smallest effort and binary impact
- ✅ Reuses `survey`/`color` already in the codebase
- ❌ Does not satisfy the actual requirement ("more visually pleasing interface," live dashboards)
- ❌ A prompt menu is not a TUI — fails the spirit of the backlog item

---

## Decision

**Chosen: Option A — Bubbletea + Lipgloss + Bubbles.**

Rationale:
1. The **Elm Model-Update-View** loop is the cleanest fit for our existing **streaming event channels** — sync/embed events become messages and render live with little glue.
2. **Bubbles** supplies the exact components we need (table, list, viewport, textinput, spinner), maximizing velocity for a solo maintainer.
3. **Lipgloss** directly delivers the "visually pleasing interface" the backlog asks for.
4. It is the **de-facto standard** for modern Go TUIs, so examples, docs, and longevity are strong.
5. The one-time learning cost of the Elm pattern and the moderate binary growth are acceptable given the feature's goals; Option C fails the requirement and Option B's channel integration and styling are weaker.

**Confirmed (2026-06-12):** the developer accepted this decision — the added binary size and new dependency cluster are a calculated cost, far outweighed by the improved user experience. The TUI ships in **`v0.3.0`** as that release's sole deliverable (resolving PRD Open Question #5).

---

## Implications

**Positives:**
- Live, reactive progress for `sync`/`embed` by consuming existing event channels as `tea.Msg`s.
- Rich, composable views (repo table, search, ask/chat, status) with consistent styling.
- Healthy ecosystem and examples reduce solo-maintainer risk.

**Negatives / Trade-offs:**
- New dependency cluster (Bubbletea/Lipgloss/Bubbles) adds binary weight to an already-large binary (ADR-006).
- Elm architecture is a paradigm shift; initial views take longer until the pattern clicks.
- A TUI is stateful UI code — more surface to test than pure CLI.

**Watch out for:**
- **Non-TTY fallback** — bare `repog` must detect no TTY (pipe/CI) and fall back to CLI/help, never block on input (PRD success criterion). Test this explicitly.
- **CLI parity** — the TUI must not become the *only* way to do anything; every action stays scriptable via subcommands.
- **Honor `NO_COLOR`** and terminal capability detection.
- **Don't duplicate business logic** — the TUI calls the same `internal/*` packages the commands do; keep it a thin presentation layer.
- **Long-running ops** must stay cancellable from the UI (Ctrl-C) and clean up (ties to ADR-008 temp-dir cleanup during deep sync — relevant once deep sync lands in V0.4.0).

> Reference this ADR from relevant code: `// See ADR-010 for why the TUI uses Bubbletea`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Developer (hackastak) | Confirmed (2026-06-12) — Bubbletea is the chosen framework; binary/dependency cost is a calculated trade-off outweighed by the UX gain; TUI ships in v0.3.0 | Decision accepted; resolves PRD Open Question #5 |
| Claude Code | Reviewed existing terminal libs (survey/spinner/color) and the event-channel design in sync/embed | Recommended Bubbletea for its fit with event streams, component ecosystem, and styling |

---

## References

- Related: [ADR-006](./ADR-006-cli-architecture-with-cobra.md) (Cobra CLI the TUI sits alongside; binary-size sensitivity)
- PRD: `~/Developer/My_Notes/1. Projects/RepoG/PRD_V0.3.0.md` (§4.3, Open Question #5)
- Backlog/notes: [[Future_Features]] (TUI + visually pleasing UI goal)
- Code: event-channel producers `internal/sync/ingest.go` (`IngestRepos`), `internal/embed/pipeline.go` (`RunEmbedPipeline`); existing terminal libs in `go.mod` (`survey/v2`, `briandowns/spinner`, `fatih/color`)
