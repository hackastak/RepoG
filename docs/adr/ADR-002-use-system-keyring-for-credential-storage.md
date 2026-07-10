# ADR-002 Use System Keyring for Credential Storage

---

## Status

`Decided`

---

## Context

RepoG requires API credentials to function:
- GitHub Personal Access Token (PAT)
- AI provider API keys (Gemini, OpenAI, OpenRouter, Anthropic, Voyage AI)
- (Optional) Ollama base URL for custom deployments

These credentials must be:
- Stored securely (not in plain text)
- Accessible without user intervention after initial setup
- Protected from accidental git commits or file sharing
- Easy to rotate or update

**Constraints:**
- CLI tool must work on macOS, Linux, and Windows
- Users should set credentials once during `repog init`
- Credentials should persist across system restarts
- Should not require root/admin privileges
- Must be accessible to the CLI without interactive prompts

**Assumptions:**
- Users are running on their personal development machines
- System keyring is available and functional on target platforms
- Users trust their OS keyring security model
- Credentials are long-lived (not rotated frequently)

---

## Evaluation Criteria

| Criterion | Weight | Notes |
|---|---|---|
| Security | High | Credentials must not be exposed in plain text |
| Developer Experience | High | One-time setup, no repeated password prompts |
| Cross-platform Support | High | Works on macOS, Linux, Windows |
| Accidental Exposure Risk | High | Prevents commits to git, file sharing |
| Operational Complexity | Medium | Should "just work" without setup |
| Auditability | Low | Ability to inspect stored credentials |

---

## Options

### Option A: System Keyring (macOS Keychain, Linux Secret Service, Windows Credential Manager)

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Security | ★★★ | OS-level encryption, protected by user login |
| Developer Experience | ★★★ | One-time setup via `repog init`, automatic retrieval |
| Cross-platform Support | ★★★ | Unified API via `zalando/go-keyring` library |
| Accidental Exposure Risk | ★★★ | Never written to disk as plain text |
| Operational Complexity | ★★★ | Zero-config, uses existing OS infrastructure |
| Auditability | ★★☆ | Viewable via OS tools (Keychain Access, etc.) |

**Trade-offs:**
- ✅ Industry-standard approach for credential storage
- ✅ Encrypted at rest by OS
- ✅ Automatically protected by user authentication
- ✅ Cannot accidentally commit credentials to git
- ✅ Works seamlessly via `zalando/go-keyring` library
- ❌ Credentials not portable across machines (by design)
- ❌ Requires functional keyring (headless servers may need alternatives)

---

### Option B: Environment Variables

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Security | ★☆☆ | Plain text in shell history, process env |
| Developer Experience | ★☆☆ | Must set variables in every shell session |
| Cross-platform Support | ★★★ | Universal support |
| Accidental Exposure Risk | ★☆☆ | Easy to expose via `.bashrc` commits, screenshots |
| Operational Complexity | ★★☆ | Simple but requires shell configuration |
| Auditability | ★★★ | Easy to inspect via `printenv` |

**Trade-offs:**
- ✅ Simple, universally supported
- ✅ Works in CI/CD and automation
- ✅ Easy to inspect and debug
- ❌ Plain text in shell history and process listings
- ❌ Must set in every shell session (or add to dotfiles)
- ❌ Frequently committed to git accidentally
- ❌ Visible to all processes on the system

---

### Option C: Config File (`~/.repog/config.yaml` with credentials)

| Criterion | Score (★★★ = high) | Notes |
|---|---|---|
| Security | ★☆☆ | Plain text on disk, world-readable if misconfigured |
| Developer Experience | ★★★ | One-time setup, always available |
| Cross-platform Support | ★★★ | File-based, works everywhere |
| Accidental Exposure Risk | ★☆☆ | Can be committed to git, shared accidentally |
| Operational Complexity | ★★★ | Simple file read/write |
| Auditability | ★★★ | Easy to inspect with text editor |

**Trade-offs:**
- ✅ Simple implementation (just file I/O)
- ✅ Portable across machines (copy config file)
- ✅ Easy to inspect and edit manually
- ❌ Plain text credentials on disk
- ❌ Can be accidentally committed to version control
- ❌ Vulnerable to file sharing, backups, log scraping
- ❌ Requires correct file permissions to prevent exposure

---

## Decision

We chose **Option A (System Keyring)** because it scores highest against our top priorities — security, developer experience, and accidental exposure prevention — and its non-portability is acceptable given:

1. **Security by default**: Credentials encrypted at rest by OS, protected by user authentication
2. **Excellent UX**: One-time setup during `repog init`, automatic retrieval thereafter
3. **Prevents accidents**: Impossible to commit credentials to git or expose in dotfiles
4. **Cross-platform**: `zalando/go-keyring` provides unified API across macOS/Linux/Windows
5. **Zero-config**: Uses existing OS infrastructure, no setup required
6. **Industry standard**: Same approach used by Docker, Kubernetes kubectl, AWS CLI

The non-portability trade-off is acceptable because:
- RepoG is designed for individual developer machines (not shared/CI environments)
- Users can re-run `repog init` on new machines easily
- Security benefits outweigh convenience of portable credentials

For CI/CD or headless environments, users can fall back to environment variables via a future enhancement.

---

## Implications

**Positives:**
- Credentials stored securely with OS-level encryption
- Cannot accidentally commit credentials to git repositories
- One-time setup provides excellent developer experience
- Works seamlessly across macOS, Linux, Windows
- Credentials protected by user login authentication
- No additional dependencies or configuration required

**Negatives / Trade-offs:**
- Credentials not portable across machines (must re-run `repog init`)
- Requires functional system keyring (may fail in Docker, WSL, headless servers)
- Cannot easily script credential setup for automation
- Debugging credential issues requires OS-specific tools (Keychain Access, etc.)
- Users cannot manually inspect credentials without OS tools

**Watch out for:**
- Headless servers or CI/CD: Keyring may not be available, need env var fallback
- WSL2 on Windows: Keyring integration may require additional setup
- Docker containers: No keyring available, need alternative approach
- If users report "keyring not available" errors, provide env var workaround
- Consider adding `--api-key` flags for non-interactive use cases

> Reference this ADR from relevant code: `// See ADR-002 for why we use system keyring for credentials`

---

## Consultation

| Stakeholder | Input | Impact on Decision |
|---|---|---|
| Security best practices | Never store credentials in plain text; use OS-provided secure storage | Confirmed keyring is the right approach |
| CLI tool precedents | Docker, kubectl, AWS CLI all use system keyring | Validated this is standard practice |
| Target users (developers) | Want simple setup, don't want to manage credential files | Favored zero-config approach |

---

## References

- Related ADRs: None
- Library: [`zalando/go-keyring`](https://github.com/zalando/go-keyring) - Cross-platform keyring access
- Supporting code: `internal/config/config.go` - Credential storage and retrieval
- Security: [SECURITY.md](../../SECURITY.md) - Security policy
- Platform-specific:
  - macOS: Uses Keychain via `security` command
  - Linux: Uses Secret Service API (GNOME Keyring, KWallet)
  - Windows: Uses Windows Credential Manager
