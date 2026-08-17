# ADR-004 Dynamic Chunking Based on Model Token Limits

---

## Status

`Decided`

---

## Context

RepoG chunks repository files into smaller pieces for embedding. Different embedding models have different token limits:

| Provider/Model | Token Limit | Character Estimate |
|----------------|-------------|-------------------|
| OpenAI `text-embedding-3-*` | 8,191 tokens | ~32,764 chars |
| Voyage AI `voyage-code-3` | 16,000 tokens | ~64,000 chars |
| Voyage AI `voyage-3` | 32,000 tokens | ~128,000 chars |
| Gemini `gemini-embedding-2-preview` (default) | 8,192 tokens | ~32,768 chars |
| Gemini `gemini-embedding-001` / `text-embedding-004` | 2,048 tokens | ~8,192 chars |
| Ollama (varies by model) | 256–32,768 tokens | ~768–98,304 chars |

**Problem:** Fixed chunk sizes cause issues:
- Too large: Embedding API rejects chunks exceeding token limit (embedding failures)
- Too small: Inefficient use of model capacity, more API calls, higher latency
- One-size-fits-all: Cannot optimize for each model's capabilities

**Why it matters:**
- Users experience "token limit exceeded" errors with fixed chunks
- Under-utilizing high-capacity models (Voyage AI) wastes opportunity
- Over-utilizing low-capacity models (Gemini) causes failures

**Constraints:**
- Must work across all supported embedding providers
- Chunk size affects storage and search quality
- Changing chunk size requires re-syncing all repositories
- Character-to-token ratio varies by content (code vs. text, languages)

**Assumptions:**
- Average char-to-token ratio is ~4:1 for code (empirical observation)
- Using 90% of token limit provides safe margin for variance
- Conservative estimation (3:1 ratio) prevents edge cases
- Users prefer automatic optimization over manual configuration

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Reliability | High | Must prevent embedding failures |
| Efficiency | High | Maximize use of model capacity |
| User Experience | High | Should be automatic, no config needed |
| Safety Margin | High | Must handle worst-case scenarios |
| Flexibility | Medium | Support future models easily |
| Implementation Complexity | Medium | Prefer simple calculation |

---

## Options

### Option A: Dynamic Chunking with Conservative Formula (Chosen)

**Formula:** `(maxTokens * 0.90) * 3 = maxChars`
- Uses 90% of token limit for safety margin
- Conservative 3:1 char-to-token ratio

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Reliability | ★★★ | 10% buffer + conservative ratio prevents failures |
| Efficiency | ★★☆ | Good utilization, some headroom left |
| User Experience | ★★★ | Fully automatic, no user configuration |
| Safety Margin | ★★★ | Handles special chars, URLs, mixed languages |
| Flexibility | ★★★ | Works with any model by reading token limit |
| Implementation Complexity | ★★★ | Simple arithmetic, no ML needed |

**Trade-offs:**
- ✅ Never exceeds token limits (no API failures)
- ✅ Automatic optimization per model
- ✅ Adapts to any future model automatically
- ✅ Simple implementation (one formula)
- ❌ Slightly conservative (could use ~10% more capacity)
- ❌ Requires re-sync when switching providers with different limits

**Example calculations:**
```
OpenAI (8,191 tokens):                            8191 * 0.90 * 3 = 22,113 chars
Gemini gemini-embedding-2-preview (8,192 tokens): 8192 * 0.90 * 3 = 22,116 chars
Gemini gemini-embedding-001 (2,048 tokens):       2048 * 0.90 * 3 =  5,529 chars
Voyage-3 (32K tokens):                           32000 * 0.90 * 3 = 86,400 chars
```

> Note: the formula uses Go integer truncation — `safeTokens := int(maxTokens * 0.90)`, then `* 3`. So `gemini-embedding-2-preview` yields `int(8192*0.90)=7372`, `7372*3 = 22,116` chars. See `CalculateMaxCharsFromTokens` in `internal/sync/ingest.go`.

---

### Option B: Fixed Chunk Size for All Models

**Approach:** Use smallest common denominator (e.g., 5,000 chars)

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Reliability | ★★★ | Safe for all models |
| Efficiency | ★☆☆ | Severely under-utilizes high-capacity models |
| User Experience | ★★★ | Simple, predictable |
| Safety Margin | ★★★ | Very conservative |
| Flexibility | ★☆☆ | Cannot leverage model improvements |
| Implementation Complexity | ★★★ | Trivial (single constant) |

**Trade-offs:**
- ✅ Simple implementation
- ✅ No re-sync when switching providers
- ✅ Predictable chunk sizes
- ❌ Wastes 90%+ of Voyage AI's capacity (86K → 5K)
- ❌ Still need to handle model-specific limits
- ❌ More chunks = more API calls = slower embedding

---

### Option C: Aggressive Chunking with Runtime Token Counting

**Approach:** Use actual tokenizer to count tokens, use 95% of limit

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Reliability | ★★☆ | Tight margins risk edge cases |
| Efficiency | ★★★ | Maximum utilization of capacity |
| User Experience | ★★☆ | Complex, requires tokenizer dependencies |
| Safety Margin | ★☆☆ | 5% buffer may not cover variance |
| Flexibility | ★☆☆ | Requires tokenizer for each provider |
| Implementation Complexity | ★☆☆ | Complex: integrate tokenizers, handle errors |

**Trade-offs:**
- ✅ Maximizes use of token capacity
- ✅ Precise token counting
- ❌ Requires provider-specific tokenizers (tiktoken, sentencepiece, etc.)
- ❌ Performance overhead (tokenize before chunking)
- ❌ Tight margins risk API failures on edge cases
- ❌ Tokenizers may not match API exactly

---

## Decision

We chose **Option A (Dynamic Chunking with Conservative Formula)** because it scores highest against our top priorities — reliability, efficiency, and user experience — and its slight over-conservatism is acceptable given:

1. **Zero embedding failures**: 10% buffer + 3:1 ratio handles worst-case content (URLs, special chars, multilingual)
2. **Automatic optimization**: Each model gets appropriate chunk size without user configuration
3. **Simple implementation**: Single formula works for all providers, no tokenizer dependencies
4. **Future-proof**: New models automatically get optimal chunk sizes
5. **Empirically validated**: Formula tested across real codebases without failures

The efficiency trade-off (using ~75% vs. ~95% of capacity) is acceptable because:
- Reliability matters more than marginal efficiency gains
- Token counting overhead would negate efficiency benefits
- Conservative chunks still provide 4-15x variation across models (5K to 86K)
- Preventing failures improves user experience more than 20% capacity gain

---

## Implications

**Positives:**
- Users never experience "token limit exceeded" errors
- Automatically optimizes for each model's capabilities
- Voyage AI `voyage-3` users get ~4x larger chunks than default Gemini users (86K vs. 22K), and up to ~15x vs. 2,048-token models (86K vs. 5K)
- Simple codebase: no tokenizer integration, one arithmetic formula
- Future models automatically supported (read token limit from provider metadata)
- Clear user feedback when chunk size changes require re-sync

**Negatives / Trade-offs:**
- Slightly conservative (leaves ~10-25% capacity unused)
- Changing providers with different token limits requires re-sync
- Cannot handle edge cases where content is >90% special characters
- Formula assumes code/text mix (may over-estimate for pure ASCII)

**Watch out for:**
- If users report embedding failures, investigate token counting mismatch
- If chunk sizes are too conservative, adjust formula to 0.85 buffer or 3.5:1 ratio
- Monitor Voyage AI usage: large chunks (86K) may hit other limits (memory, latency)
- If providers change token limits, ensure metadata updates propagate
- Consider caching chunk size calculation to avoid recalculating per file

> Reference this ADR from relevant code: `// See ADR-004 for dynamic chunking formula`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| User reports | "Embedding failed: token limit exceeded" with fixed chunks | Confirmed need for dynamic sizing |
| Provider docs | OpenAI: 8191, Gemini: 2048, Voyage: 16K-32K | Validated wide variance in limits |
| Empirical testing | 3:1 ratio prevents failures across test repos | Confirmed conservative ratio works |
| Developer (hackastak) | Prefer simple solution over tokenizer complexity | Favored formula approach |

---

## References

- Related ADRs:
  - [[ADR-003]] - Uses similar clear-on-change pattern when chunk size changes
- Supporting docs:
  - [chunking.md](../chunking.md) - Detailed explanation and examples
- Supporting code:
  - `internal/sync/ingest.go` - `CalculateMaxCharsFromTokens` (chunk size formula) and `splitContent`
  - `internal/provider/*/embedding.go` - per-model `MaxTokens` metadata (gemini, openai, openrouter, voyageai, ollama)
- Provider documentation:
  - [OpenAI Embeddings](https://platform.openai.com/docs/guides/embeddings) - 8,191 token limit
  - [Voyage AI Docs](https://docs.voyageai.com/) - 16K and 32K limits
  - [Gemini API](https://ai.google.dev/gemini-api/docs/embeddings) - 2,048 token limit
