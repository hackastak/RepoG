# RepoG

AI-powered knowledge base for your GitHub repositories.

## Overview

RepoG is a CLI-first, local AI-powered tool that helps you explore, search, and understand your GitHub repositories using semantic search and LLM-powered insights.

## Prerequisites

- Node.js >= 18.0.0
- pnpm >= 8.0.0
- GitHub Personal Access Token (with `repo` and `read:user` scopes)
- Google Gemini API Key

## Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/your-username/repog.git
   cd repog
   ```

2. Install dependencies:
   ```bash
   pnpm install
   ```

3. Build all packages:
   ```bash
   pnpm build
   ```

4. Link the CLI globally (optional):
   ```bash
   cd packages/cli && pnpm link --global
   ```

## Usage

```bash
# Initialize RepoG with your credentials
repog init

# Sync your GitHub repositories
repog sync

# Generate embeddings for semantic search
repog embed

# Search your repositories
repog search "authentication middleware"

# Get recommendations
repog recommend "I need a project with good testing practices"

# Ask questions about your codebase
repog ask "How does the auth system work in my-project?"

# Summarize a repository
repog summarize owner/repo

# Check sync status
repog status
```

## Project Structure

```
repog/
├── packages/
│   ├── core/    # Core library: database, auth, sync, embed, search, RAG
│   └── cli/     # Command-line interface
```

## Development

```bash
# Run in development mode (watch)
pnpm dev

# Run tests
pnpm test

# Lint code
pnpm lint

# Clean build artifacts
pnpm clean
```

## License

MIT
