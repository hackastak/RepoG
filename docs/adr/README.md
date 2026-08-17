# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the RepoG project.

## What are ADRs?

Architecture Decision Records document important architectural decisions made in this project. They capture:
- **Why** a decision was made (context and constraints)
- **What** alternatives were considered
- **How** the decision was evaluated
- **What** the implications are (both positive and negative)

ADRs are a gift to your future self and other developers. They prevent the "why does our code work this way?" questions and preserve institutional knowledge.

## When to Write an ADR

Create an ADR when making decisions about:
- **Architecture patterns**: Microservices vs. monolith, event-driven vs. request/response
- **Technology choices**: Database selection, external service providers, frameworks
- **Testing strategies**: What to test, how to test it, coverage expectations
- **Quality attributes**: Performance trade-offs, security patterns, reliability approaches
- **Standards and conventions**: Code organization, error handling, API design

**Don't** write ADRs for:
- Routine implementation details
- Obvious choices with no meaningful alternatives
- Decisions that can be easily reversed without cost

## How to Write an ADR

1. Copy the template from `~/Developer/My_Notes/3. Resources/Software_Engineering/Systems_Architecture/ADR_Template.md`
2. Number sequentially: `ADR-XXX-descriptive-name.md`
3. Use the decision as the title, not the topic (e.g., "Use PostgreSQL for Media Service" not "Database Selection")
4. Fill in all sections:
   - **Status**: Draft → Decided → Superseded
   - **Context**: Why this decision matters
   - **Evaluation Criteria**: What you're optimizing for
   - **Options**: 2-3 options with trade-off analysis
   - **Decision**: The chosen option and why
   - **Implications**: Both positives AND negatives
   - **Consultation**: Who was involved
   - **References**: Links to related ADRs or docs

5. Reference the ADR in code where relevant: `// See ADR-XXX for why we...`

## ADR Status Lifecycle

- **Draft**: Under discussion, not yet finalized
- **Decided**: Approved and currently in effect
- **Deferred to vX.Y.Z**: Decision/analysis stands, but implementation is scheduled for a later release
- **Superseded by ADR-XXX**: Replaced by a newer decision

## Index of ADRs

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [ADR-001](./ADR-001-use-sqlite-with-sqlite-vec-for-vector-storage.md) | Use SQLite with sqlite-vec for Vector Storage | Decided | 2026-04-04 |
| [ADR-002](./ADR-002-use-system-keyring-for-credential-storage.md) | Use System Keyring for Credential Storage | Decided | 2026-04-04 |
| [ADR-003](./ADR-003-clear-on-change-strategy-for-embedding-migrations.md) | Clear-on-Change Strategy for Embedding Migrations | Decided | 2026-04-04 |
| [ADR-004](./ADR-004-dynamic-chunking-based-on-model-token-limits.md) | Dynamic Chunking Based on Model Token Limits | Decided | 2026-04-04 |
| [ADR-005](./ADR-005-factory-pattern-with-self-registration-for-providers.md) | Factory Pattern with Self-Registration for Providers | Decided | 2026-04-04 |
| [ADR-006](./ADR-006-cli-architecture-with-cobra.md) | CLI Architecture with Cobra | Decided | 2026-04-04 |
| [ADR-007](./ADR-007-use-factory-tests-for-provider-packages.md) | Use Factory Tests for Provider Package Testing Strategy | Decided | 2026-04-04 |
| [ADR-008](./ADR-008-shell-to-system-git-for-deep-sync-cloning.md) | Shell to System Git for Deep-Sync Cloning | Decided — deferred to V0.5.0 | 2026-06-10 |
| [ADR-009](./ADR-009-size-bounded-line-aware-chunking-for-code.md) | Size-Bounded, Line-Aware Chunking for Source Code | Deferred to V0.5.0 | 2026-06-10 |
| [ADR-010](./ADR-010-use-bubbletea-for-the-tui.md) | Use Bubbletea for the Interactive TUI | Decided (V0.3.0) | 2026-06-12 |
| [ADR-011](./ADR-011-cancel-and-suspend-for-interactive-sync-and-embed.md) | Cancel and Suspend for Interactive Sync and Embed Runs | Decided | 2026-08-06 |

## References

- Template source: `~/Developer/My_Notes/3. Resources/Software_Engineering/Systems_Architecture/ADR_Template.md`
- Inspiration: [Why Does Our Code Work This Way? — Nobody Knows. ADRs Fix That.](https://medium.com/codetodeploy/why-does-our-code-work-this-way-nobody-knows-adrs-fix-that-ea938a3670ad) by Alina Kovtun
