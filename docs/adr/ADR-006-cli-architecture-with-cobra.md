# ADR-006 CLI Architecture with Cobra

---

## Status

`Decided`

---

## Context

RepoG is a command-line tool with multiple subcommands:
- `repog init` - Configuration setup
- `repog sync` - Repository synchronization
- `repog embed` - Generate embeddings
- `repog search <query>` - Semantic search
- `repog ask <question>` - RAG-based Q&A
- `repog recommend <task>` - Repository recommendations
- `repog summarize <repo>` - AI summarization
- `repog status` - Statistics
- `repog reconfig` - Reconfiguration

**Requirements:**
- Clean command structure with subcommands
- Flag parsing (global and command-specific)
- Help text generation
- Command aliasing and shortcuts
- Interactive prompts for user input
- Persistent flags (e.g., `--verbose` across all commands)

**Constraints:**
- Pure Go implementation (no shell scripts)
- Cross-platform support (macOS, Linux, Windows)
- Standard CLI conventions (--help, --version, etc.)
- Must integrate with Go's flag parsing

**Assumptions:**
- Users familiar with modern CLI tools (kubectl, git, docker)
- Standard CLI patterns preferred over custom approaches
- Help text should be automatically generated

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Developer Experience | High | Easy to add new commands |
| Standard Patterns | High | Familiar to CLI users |
| Maintainability | High | Clear code organization |
| Feature Completeness | Medium | Flags, help, validation |
| Community Support | Medium | Active library, good docs |
| Bundle Size | Low | CLI tool size acceptable |

---

## Options

### Option A: Cobra (Chosen)

**Approach:** Use `spf13/cobra` for command structure; configuration is handled by a custom keyring/YAML layer (see ADR-002), not Viper

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Experience | ★★★ | Declarative command definitions, auto-generated help |
| Standard Patterns | ★★★ | Industry standard (kubectl, gh, hugo use it) |
| Maintainability | ★★★ | Clear separation: one file per command |
| Feature Completeness | ★★★ | Flags, persistent flags, aliases, validation, completion |
| Community Support | ★★★ | Widely used, excellent documentation |
| Bundle Size | ★★☆ | ~2MB for full framework |

**Trade-offs:**
- ✅ Industry-standard choice (used by Kubernetes, GitHub CLI, Hugo)
- ✅ Auto-generated help text and flag parsing
- ✅ Built-in shell completion support
- ✅ Persistent flags (global options)
- ✅ Command grouping and aliases
- ✅ Extensive documentation and examples
- ❌ Adds ~2MB to binary size
- ❌ Cobra's scaffolding is more than a tiny CLI strictly needs

**Example:**
```go
var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Sync repository metadata and content",
    Long:  `Syncs owned and starred repositories...`,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation
    },
}

func init() {
    rootCmd.AddCommand(syncCmd)
    syncCmd.Flags().BoolP("owned", "o", false, "Sync owned repos only")
    syncCmd.Flags().BoolP("starred", "s", false, "Sync starred repos only")
}
```

---

### Option B: Standard library flag package

**Approach:** Use `flag` package with manual subcommand parsing

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Experience | ★☆☆ | Manual parsing, verbose boilerplate |
| Standard Patterns | ★★☆ | Familiar but limited |
| Maintainability | ★☆☆ | Lots of manual switch statements |
| Feature Completeness | ★☆☆ | Basic flags only, no help generation |
| Community Support | ★★★ | Standard library, stable |
| Bundle Size | ★★★ | Minimal (stdlib only) |

**Trade-offs:**
- ✅ No external dependencies
- ✅ Minimal binary size
- ✅ Standard library stability
- ❌ No subcommand support (must implement manually)
- ❌ No auto-generated help text
- ❌ Verbose flag parsing code
- ❌ No persistent flags or aliases

**Example:**
```go
func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: repog <command>")
        return
    }

    switch os.Args[1] {
    case "sync":
        syncCmd := flag.NewFlagSet("sync", flag.ExitOnError)
        owned := syncCmd.Bool("owned", false, "Sync owned repos")
        syncCmd.Parse(os.Args[2:])
        // Run sync...
    case "search":
        // ...
    }
}
```

---

### Option C: cli (urfave/cli)

**Approach:** Alternative CLI framework

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Developer Experience | ★★☆ | Good, but different patterns than Cobra |
| Standard Patterns | ★★☆ | Less common than Cobra in modern tools |
| Maintainability | ★★☆ | Slice-based command definitions |
| Feature Completeness | ★★★ | Full-featured, similar to Cobra |
| Community Support | ★★☆ | Good but smaller than Cobra |
| Bundle Size | ★★☆ | Similar to Cobra |

**Trade-offs:**
- ✅ Similar features to Cobra
- ✅ Simpler API in some ways
- ❌ Less widely adopted than Cobra
- ❌ Different patterns from kubectl/gh (learning curve for contributors)
- ❌ Similar bundle size to Cobra

---

## Decision

We chose **Option A (Cobra)** because it scores highest against our top priorities — developer experience, standard patterns, and feature completeness — and its bundle size is acceptable given:

1. **Industry standard**: Used by kubectl, GitHub CLI, Hugo, and hundreds of popular Go CLI tools
2. **Developer productivity**: Declarative commands, auto-generated help, minimal boilerplate
3. **Feature-rich**: Persistent flags, shell completion, aliases, command grouping all built-in
4. **Maintainability**: One file per command, clear structure
5. **Community**: Excellent documentation, large ecosystem, active maintenance
6. **Familiar patterns**: Contributors and users already know Cobra conventions

The bundle size trade-off (~2MB) is acceptable because:
- RepoG already includes SQLite and AI provider SDKs (~15-20MB total)
- 2MB for full CLI framework is negligible in context
- Developer productivity and user experience benefits outweigh size concerns
- Modern systems have ample disk space for CLI tools

---

## Implications

**Positives:**
- Commands are self-documenting via auto-generated help text
- Adding new commands is straightforward (create file, define command, register)
- Persistent flags work across all commands (`--verbose`, `--config`)
- Shell completion support (bash, zsh, fish, powershell)
- Flag validation and required flags built-in
- Clear code organization (one file per command in `commands/`)
- Users get familiar CLI patterns (same as kubectl, gh, etc.)

**Negatives / Trade-offs:**
- Adds ~2MB to binary size
- Learning curve for developers unfamiliar with Cobra
- Configuration is intentionally handled by a custom keyring/YAML layer (see ADR-002) rather than Viper
- Must follow Cobra conventions (can't easily deviate)

**Watch out for:**
- Cobra's validation runs before RunE - ensure flag validation happens early
- Persistent flags must be defined on rootCmd, not subcommands
- Help text should be concise but complete (users rely on it)
- Consider adding examples to help text for complex commands
- Shell completion needs to be generated and distributed with releases

> Reference this ADR from relevant code: `// See ADR-006 for CLI framework choice`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Go community | Cobra is the de facto standard for Go CLIs | Strong signal for adoption |
| Kubernetes project | kubectl uses Cobra extensively | Validated scalability and robustness |
| Developer (hackastak) | Want standard patterns, easy to extend | Favored Cobra over custom solution |

---

## References

- Related ADRs: None
- Library: [`spf13/cobra`](https://github.com/spf13/cobra) - CLI framework
- Configuration: custom keyring/YAML layer (see ADR-002) — Viper is intentionally not used
- Supporting code:
  - `cmd/repog/main.go` - CLI entry point
  - `commands/*.go` - Individual command implementations
  - `commands/root.go` - Root command and persistent flags
- Tools using Cobra:
  - [kubectl](https://kubernetes.io/docs/reference/kubectl/) - Kubernetes CLI
  - [gh](https://cli.github.com/) - GitHub CLI
  - [hugo](https://gohugo.io/) - Static site generator
  - [docker](https://docs.docker.com/engine/reference/commandline/cli/) - Container platform
