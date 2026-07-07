# ADR-007 Use Factory Tests for Provider Package Testing Strategy

---

## Status

`Superseded` — the original decision (Option A, factory tests only) was reversed during the post-0.3.0 "Go rewrite remediation." Comprehensive httptest-based unit tests (Option B) were subsequently added to all six provider subpackages. See the [Superseding update](#superseding-update-option-b-adopted) below; the rest of this ADR is retained as a historical record of the original reasoning.

---

## Superseding update: Option B adopted

The pragmatic bet below — that thin HTTP wrappers did not warrant unit tests — did not hold. During the post-0.3.0 remediation the six provider subpackages (`anthropic`, `gemini`, `openai`, `openrouter`, `ollama`, `voyageai`), each previously at 0% coverage, gained white-box unit tests using `httptest`-mocked HTTP servers covering success paths, non-200 error handling, model fallback, and streaming. Per-package coverage now ranges roughly 78–91%, and total statement coverage rose from 34.9% to 54.4%.

Consequences of this reversal versus the "Implications" recorded below:
- HTTP error handling, fallback logic, and streaming behavior **are** now automatically tested.
- Provider subpackages are no longer reported at 0% coverage.
- The maintenance burden the ADR sought to avoid (mock fixtures tracking API changes) was accepted as worthwhile.

The remaining historical sections describe the original Option A decision and are kept for context.

---

## Context

During CI setup for macOS testing with race detection and coverage collection, we discovered that provider subdirectories (`internal/provider/anthropic`, `internal/provider/gemini`, `internal/provider/ollama`, `internal/provider/openai`, `internal/provider/openrouter`, `internal/provider/voyageai`) contain substantial implementation code but no dedicated unit tests. This caused the Go `covdata` tool to fail when attempting to merge coverage profiles.

These packages contain:
- HTTP client implementations for external AI APIs
- Request/response marshaling logic
- Error handling and fallback mechanisms
- Streaming response processing
- Model configuration and defaults

**Constraints:**
- Need CI to pass on both Linux and macOS
- Want to maintain reasonable test coverage metrics
- Limited development time for test infrastructure
- Providers are thin wrappers around external HTTP APIs

**Assumptions:**
- Factory pattern tests provide sufficient coverage for registration and basic instantiation
- Real bugs in provider implementations would surface during integration/manual testing
- The complexity of mocking HTTP responses outweighs immediate value of unit tests
- Provider implementations are stable and change infrequently

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Developer Velocity | High | Time to ship features vs. time writing mocks |
| Test Coverage | Medium | Coverage percentage and risk mitigation |
| Maintenance Burden | High | Effort to maintain test infrastructure |
| CI Reliability | High | Tests must pass consistently on all platforms |
| Regression Detection | Medium | Ability to catch breaking changes |
| Code Confidence | Medium | Developer confidence when refactoring |

---

## Options

### Option A: Factory Tests Only (Pragmatic Approach)

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Velocity | ★★★ | No additional test writing required |
| Test Coverage | ★☆☆ | Factory tests verify registration and instantiation only |
| Maintenance Burden | ★★★ | Minimal - existing factory_test.go already works |
| CI Reliability | ★★★ | Eliminates covdata errors, CI passes reliably |
| Regression Detection | ★☆☆ | Won't catch logic bugs in HTTP handling or streaming |
| Code Confidence | ★★☆ | Reasonable confidence for thin API wrappers |

**Trade-offs:**
- ✅ Ships quickly without test infrastructure overhead
- ✅ Factory tests verify core integration pattern works
- ❌ No coverage for error handling, fallback logic, or streaming edge cases
- ❌ HTTP client bugs would only be caught in production/manual testing

---

### Option B: Comprehensive Unit Tests with HTTP Mocking

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Velocity | ★☆☆ | Significant upfront investment in httptest infrastructure |
| Test Coverage | ★★★ | Would test all code paths, error handling, streaming |
| Maintenance Burden | ★☆☆ | Must maintain mock responses as APIs evolve |
| CI Reliability | ★★★ | Fully isolated tests, no external dependencies |
| Regression Detection | ★★★ | Catches bugs in marshaling, fallback, error handling |
| Code Confidence | ★★★ | High confidence when refactoring providers |

**Trade-offs:**
- ✅ Comprehensive coverage of all code paths
- ✅ Documents expected API behavior through test fixtures
- ❌ High initial cost: ~6 packages × ~200 lines of test code each
- ❌ Requires httptest.Server boilerplate and mock response fixtures
- ❌ Mocks may diverge from actual API behavior

---

### Option C: Integration Tests Against Live APIs

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Velocity | ★★☆ | Faster than mocking, but requires API key management |
| Test Coverage | ★★★ | Tests real API interactions |
| Maintenance Burden | ★★☆ | Must manage API keys, rate limits, test isolation |
| CI Reliability | ★☆☆ | Flaky due to network, rate limits, API changes |
| Regression Detection | ★★★ | Catches real API contract changes |
| Code Confidence | ★★☆ | High confidence APIs work, but tests may be brittle |

**Trade-offs:**
- ✅ Tests against real APIs verify actual behavior
- ✅ Less maintenance than mocking (no fixture drift)
- ❌ Requires API keys in CI, cost implications
- ❌ Slow tests (network latency)
- ❌ Flaky CI due to network issues or API rate limits
- ❌ Cannot run offline

---

## Decision

We chose **Option A (Factory Tests Only)** because it scores highest against our top priorities — developer velocity, maintenance burden, and CI reliability — and its limited regression detection is acceptable given that:

1. Provider packages are thin HTTP client wrappers with straightforward logic
2. The factory tests verify the critical integration pattern (registration → instantiation → basic properties)
3. API changes are infrequent and would likely be caught during development/testing workflows
4. The cost of maintaining comprehensive mocks outweighs immediate value for stable code
5. We can revisit this decision if providers become more complex or bugs emerge

---

## Implications

**Positives:**
- CI passes reliably on all platforms (Linux, macOS) without covdata errors
- Zero additional test infrastructure or maintenance burden
- Development velocity remains high for feature work
- Factory tests provide baseline confidence in provider registration

**Negatives / Trade-offs:**
- No automated testing of HTTP error handling, fallback logic, or streaming behavior
- Coverage metrics exclude provider subdirectories (reported as 0% coverage)
- Bugs in provider implementations may only surface during manual testing or production use
- Refactoring provider code has less safety net than fully unit-tested code

**Watch out for:**
- If providers accumulate complex business logic beyond thin HTTP wrappers, revisit this decision
- If production bugs emerge in provider packages, add targeted unit tests for those scenarios
- If API contracts change frequently, consider adding contract tests
- Coverage metrics appear lower than actual risk due to untested provider packages

> Reference this ADR from relevant code: `// See ADR-007 for why provider packages use factory tests only`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Developer (hackastak) | Questioned whether unit tests are completely necessary or just best practice | Confirmed pragmatic approach is acceptable; framed decision as "good for now" with option to add tests later if needed |
| Claude Code | Analyzed provider code complexity and test coverage gaps | Provided evaluation framework showing factory tests offer reasonable coverage for thin API wrappers |

---

## References

- Related commits: CI fix to separate test execution from coverage collection
- Code: `internal/provider/factory_test.go` provides existing factory-level test coverage
- Technical context: Go covdata tool fails on macOS when using `-race -coverprofile` on packages without test files
- Supporting doc: [CLAUDE.md](../../CLAUDE.md) build and test commands
