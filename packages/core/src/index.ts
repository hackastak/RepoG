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

// Config (encrypted credential storage)
export {
  saveConfig,
  loadConfig,
  clearConfig,
  isConfigured,
  getDefaultDbPath,
  type ConfigData,
  type SaveConfigResult,
} from './config/config.js';

// Database
export { getDb, closeDb, migrate } from './db/index.js';
export * from './db/schema.js';
export * from './db/init.js';

// GitHub Auth
export * from './github/auth.js';

// Gemini Auth
export * from './gemini/auth.js';

// Legacy Auth (re-exports for backwards compatibility)
// Note: some functions have same names as config module but different signatures
export {
  getConfig,
  getGitHubToken,
  setGitHubToken,
  getGeminiApiKey,
  setGeminiApiKey,
  clearCredentials,
  // saveConfig and isConfigured are exported from config/config.js
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
