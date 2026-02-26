/**
 * @repog/core - Core library for RepoG
 *
 * This package provides the core functionality for the RepoG CLI:
 * - Database management with SQLite + sqlite-vec
 * - GitHub API integration
 * - Embeddings generation
 * - Semantic search
 * - RAG pipeline
 */

// Types
export * from './types/index.js';

// Database
export { getDb, closeDb, migrate } from './db/index.js';
export * from './db/schema.js';

// Auth
export {
  getConfig,
  saveConfig,
  getGitHubToken,
  setGitHubToken,
  getGeminiApiKey,
  setGeminiApiKey,
  isConfigured,
  clearCredentials,
} from './auth/index.js';

// Sync
export {
  syncRepos,
  syncRepo,
  getSyncState,
  fetchReadme,
  fetchFileTree,
  resumeSync,
} from './sync/index.js';

// Embed
export {
  embedRepos,
  embedRepo,
  chunkRepo,
  generateEmbedding,
  getPendingEmbedCount,
  deleteEmbeddings,
} from './embed/index.js';

// Search
export { search, searchInRepo, findSimilarRepos, logQuery } from './search/index.js';

// RAG
export {
  ask,
  recommend,
  summarize,
  buildContext,
  generateResponse,
  getRepoForContext,
} from './rag/index.js';
