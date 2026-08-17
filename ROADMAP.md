# RepoG Roadmap

This is the public, contributor-facing roadmap for RepoG. It describes the
direction of the project and what stands between today and a stable `v1.0.0`.

It is a statement of intent, not a commitment — priorities and timing can shift.
For released changes see the [Changelog](CHANGELOG.md); for day-to-day work see the
[issues page](https://github.com/hackastak/repog/issues).

**Current release:** `v0.4.0` (provider retry/backoff, TUI suspend/resume/cancel, and
hardening). RepoG is pre-`v1.0.0`, so it follows
[Semantic Versioning](https://semver.org/) with breaking changes called out in the
[Upgrade notes](docs/UPGRADING.md).

## What `v1.0.0` means

`v1.0.0` is about turning a working tool into a stable one: a settled feature set,
real test coverage, clean documentation, and broader distribution — not a SaaS launch.
RepoG stays a **local-first CLI knowledge base and RAG layer over your own GitHub repos.**

## The road to v1.0

These are the items that gate a `v1.0.0` tag.

| Area | Goal | Status |
|------|------|--------|
| **Full code-base syncing** | Deep, clone-based sync that indexes real source code (not just metadata and READMEs), with cost guardrails: opt-in `--deep`, file-size caps, and `.repogignore` support. | Next up — `v0.5.0` |
| **Incremental syncing speed** | Re-sync only what changed instead of re-ingesting everything. A hard gate: `v1.0.0` will not ship before this lands. | Planned (v1.0 gate) |
| **Embedding generation speed** | Faster embedding through better batching and parallelism. | Planned |
| **Knowledge-graph / docs export** | A command to generate documentation and a knowledge graph from indexed repos. | Planned |
| **Performance at scale** | Handle 1,000+ repos without crashing or hitting rate limits; search in < 300ms; CLI startup < 500ms. | Planned |
| **Test coverage** | Raise coverage toward the 80% target, with integration tests across all providers. | In progress |
| **Broader distribution** | Linux Homebrew distribution via CI (cross-compiling from macOS is blocked by the sqlite-vec + musl toolchain). | Planned |
| **Robustness polish** | Consistent error handling for API failures, missing config, and empty databases; rate-limit warnings and progress indicators across every provider. | Planned |

## After v1.0

Directions we're interested in once the core is stable — not scheduled, and open to
contribution:

- **Wider distribution** — Scoop (Windows) and apt/dnf (Linux distros).
- **Opt-in observability** — anonymous, explicitly opt-in usage telemetry and error
  reporting to help prioritize.
- **UX polish** — language / tech-stack usage stats in `repog status`, colorized output,
  and a smoother first-run onboarding flow.

## Out of scope

Things we've deliberately decided *not* to pursue, so contributors don't invest in them:

- **Other Git platforms** — GitLab, Bitbucket, and self-hosted Git servers. RepoG is
  GitHub-focused by design.
- **Team / collaboration / SaaS features** — shared knowledge bases, collaborative
  annotations, multi-user hosting.
- **Plugin system and internationalization.**

## Influencing the roadmap

Have a use case or disagree with a priority? Open an issue or start a discussion:

- [Request a feature](https://github.com/hackastak/repog/issues/new?template=feature_request.md)
- [Report a bug](https://github.com/hackastak/repog/issues/new?template=bug_report.md)

See [CONTRIBUTING.md](CONTRIBUTING.md) if you'd like to help build any of the above.
