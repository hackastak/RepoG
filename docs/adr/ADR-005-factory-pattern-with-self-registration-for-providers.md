# ADR-005 Factory Pattern with Self-Registration for Providers

---

## Status

`Decided`

---

## Context

RepoG supports multiple AI providers for embedding and text generation: Gemini, OpenAI, OpenRouter, and Ollama support both; Voyage AI is embedding-only; Anthropic is text-generation-only. The system needs a clean way to:

**Requirements:**
- Allow each provider to be implemented independently
- Enable adding new providers without modifying core code
- Avoid import cycles between `config` and `provider` packages
- Support provider discovery at runtime
- Make testing easy (mock providers)

**Problem:** Go package import cycles prevent straightforward dependency injection:
```
config → provider/gemini  (to create provider)
provider/gemini → config  (to read ProviderConfig)
ERROR: import cycle
```

**Why it matters:**
- Adding new providers should be plug-and-play
- Core code should be decoupled from specific providers
- Testing requires easy provider mocking
- Configuration system must remain provider-agnostic

**Constraints:**
- Go's strict import cycle detection
- Must work with blank imports (`import _ "provider/gemini"`)
- Provider implementations in separate subpackages
- Configuration must drive provider selection

**Assumptions:**
- Providers are known at compile time (not dynamic plugins)
- Registration happens during package initialization
- Users won't implement custom providers (internal extensibility only)

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Avoids Import Cycles | High | Must compile without cycles |
| Extensibility | High | Easy to add new providers |
| Testability | High | Mock providers for unit tests |
| Simplicity | High | Easy to understand and maintain |
| Type Safety | Medium | Compile-time checks where possible |
| Discoverability | Low | Easy to find available providers |

---

## Options

### Option A: Factory Pattern with Self-Registration via init() (Chosen)

**Approach:** Each provider registers itself in `init()` function

```go
// internal/provider/factory.go
var embeddingProviders = make(map[string]EmbeddingProviderFactory)

func RegisterEmbeddingProvider(name string, factory EmbeddingProviderFactory) {
    embeddingProviders[name] = factory
}

func NewEmbeddingProvider(cfg config.ProviderConfig, key string) (EmbeddingProvider, error) {
    factory, ok := embeddingProviders[cfg.Provider]
    if !ok {
        return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
    }
    return factory(cfg, key)
}

// internal/provider/gemini/embedding.go
func init() {
    provider.RegisterEmbeddingProvider("gemini", func(cfg config.ProviderConfig, key string) {
        return NewGeminiEmbeddingProvider(key, cfg.Model, cfg.Dimensions)
    })
}
```

**Usage:**
```go
import (
    "internal/provider"
    _ "internal/provider/gemini"  // Triggers init()
    _ "internal/provider/openai"
)

p, err := provider.NewEmbeddingProvider(cfg, apiKey)
```

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Avoids Import Cycles | ★★★ | Config → provider (interface), providers → config (self-register) |
| Extensibility | ★★★ | Add provider = create package + init(), no core changes |
| Testability | ★★★ | Easy to register mock providers in tests |
| Simplicity | ★★☆ | Slightly magic (init() runs implicitly) |
| Type Safety | ★★☆ | Factory functions, runtime provider lookup |
| Discoverability | ★★☆ | Must check blank imports to see available providers |

**Trade-offs:**
- ✅ No import cycles
- ✅ Adding providers requires zero changes to core code
- ✅ Clean separation: providers are self-contained
- ✅ Easy mocking for tests (register test factories)
- ✅ Similar to database driver pattern (`database/sql`)
- ❌ Slightly "magical" (init() runs implicitly)
- ❌ Provider list not obvious without reading imports

---

### Option B: Centralized Provider Registry

**Approach:** Single package imports all providers and registers them

```go
// internal/provider/registry.go
import (
    "internal/provider/gemini"
    "internal/provider/openai"
)

func init() {
    embeddingProviders["gemini"] = gemini.NewEmbeddingProvider
    embeddingProviders["openai"] = openai.NewEmbeddingProvider
}
```

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Avoids Import Cycles | ★☆☆ | Creates import cycle: registry → providers → config → registry |
| Extensibility | ★☆☆ | Must modify registry.go for each new provider |
| Testability | ★★☆ | Can mock, but registry is global |
| Simplicity | ★★★ | Explicit, easy to understand |
| Type Safety | ★★☆ | Same as Option A |
| Discoverability | ★★★ | All providers listed in one file |

**Trade-offs:**
- ✅ Explicit provider list in one place
- ✅ No init() "magic"
- ❌ Import cycles still occur
- ❌ Every new provider requires modifying registry
- ❌ Tight coupling between registry and providers

---

### Option C: Dependency Injection via Constructor

**Approach:** Pass provider implementations explicitly

```go
// commands/embed.go
func RunEmbed(embeddingProvider provider.EmbeddingProvider) {
    // ...
}

// main.go
func main() {
    cfg := config.Load()
    var embedProvider provider.EmbeddingProvider
    switch cfg.Embedding.Provider {
    case "gemini":
        embedProvider = gemini.NewEmbeddingProvider(...)
    case "openai":
        embedProvider = openai.NewEmbeddingProvider(...)
    }
    commands.RunEmbed(embedProvider)
}
```

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Avoids Import Cycles | ★★☆ | main → config, main → providers (still cycles) |
| Extensibility | ★☆☆ | Must modify main.go switch for each provider |
| Testability | ★★★ | Excellent: pass mock providers directly |
| Simplicity | ★★★ | Explicit, no magic |
| Type Safety | ★★★ | Full compile-time checking |
| Discoverability | ★★★ | Switch statement shows all providers |

**Trade-offs:**
- ✅ Explicit dependencies
- ✅ Great for testing (inject mocks)
- ✅ No global state
- ❌ Still has import cycle issues
- ❌ Boilerplate in every command
- ❌ Must thread providers through command stack

---

## Decision

We chose **Option A (Factory Pattern with Self-Registration)** because it scores highest against our top priorities — avoiding import cycles, extensibility, and testability — and its implicit initialization is acceptable given:

1. **Solves import cycles**: Config doesn't import providers; providers register themselves
2. **Zero-touch extensibility**: Adding a provider = create package + blank import, no core code changes
3. **Standard Go pattern**: Similar to `database/sql` driver registration (familiar to Go developers)
4. **Clean architecture**: Providers are self-contained, no centralized coupling
5. **Easy testing**: Register mock providers in test `init()` functions

The "magic" trade-off of implicit `init()` is acceptable because:
- This is idiomatic Go for plugin-like systems (database drivers, image formats, etc.)
- Clear documentation explains the pattern
- Benefits of decoupling outweigh discoverability concerns
- Blank imports make provider dependencies visible

---

## Implications

**Positives:**
- No import cycles in the codebase
- Adding new providers requires zero changes to core code
- Providers are self-contained modules
- Easy to test with mock providers
- Follows established Go patterns (database/sql)
- Config system remains provider-agnostic
- Clear separation of concerns

**Negatives / Trade-offs:**
- `init()` functions run implicitly (slightly "magical")
- Available providers not obvious without checking blank imports
- Runtime provider lookup (not compile-time)
- Global registry state (shared across tests unless reset)
- Must remember to add blank import when adding provider

**Watch out for:**
- Test isolation: Reset provider registry between tests if needed
- Forgotten blank imports: Provider won't be available if import missing
- Provider name conflicts: Ensure unique provider names
- Documentation: Clearly document how to add new providers
- Consider adding provider list validation during config load

> Reference this ADR from relevant code: `// See ADR-005 for provider registration pattern`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Go best practices | database/sql uses identical pattern for drivers | Validated this is idiomatic Go |
| Developer (hackastak) | Import cycles blocking implementation | Confirmed need for self-registration |
| Code reviewers | Pattern is familiar from database drivers | Reduced learning curve concern |

---

## References

- Related ADRs: None
- Supporting code:
  - `internal/provider/factory.go` - Factory functions and registry
  - `internal/provider/*/embedding.go` - Provider init() functions
  - `internal/provider/*/llm.go` - Provider init() functions
  - `internal/provider/factory_test.go` - Tests for registration
- Go patterns:
  - [`database/sql`](https://pkg.go.dev/database/sql) - Driver registration pattern
  - [`image`](https://pkg.go.dev/image) - Format registration pattern
- Alternative considered: [Wire](https://github.com/google/wire) - Compile-time dependency injection (too heavy for this use case)
