/**
 * Core domain types for RepoG
 */

/**
 * Represents a GitHub repository stored in the local database.
 * Matches the `repos` table schema.
 */
export interface Repo {
  id: number;
  github_id: number;
  owner: string;
  name: string;
  full_name: string;
  description: string | null;
  url: string;
  homepage: string | null;
  primary_language: string | null;
  topics: string | null;
  stars: number;
  is_fork: number;
  is_private: number;
  is_starred: number;
  is_owned: number;
  readme: string | null;
  file_tree: string | null;
  pushed_at: string;
  synced_at: string;
  embedded_at: string | null;
  pushed_at_hash: string | null;
  created_at: string;
}

/**
 * Represents a text chunk extracted from repository content.
 * Matches the `chunks` table schema.
 */
export interface Chunk {
  id: number;
  repo_id: number;
  chunk_type: string;
  chunk_index: number;
  content: string;
  token_estimate: number | null;
  created_at: string;
}

/**
 * Represents the state of a sync operation.
 * Matches the `sync_state` table schema.
 */
export interface SyncState {
  id: number;
  sync_type: string;
  status: string;
  total_repos: number;
  processed_repos: number;
  last_cursor: string | null;
  started_at: string;
  completed_at: string | null;
  error: string | null;
}

/**
 * Represents a logged query for analytics.
 * Matches the `query_log` table schema.
 */
export interface QueryLog {
  id: number;
  query_type: string;
  query_text: string;
  repo_full_name: string | null;
  results_count: number | null;
  latency_ms: number | null;
  created_at: string;
}

/**
 * Configuration stored in ~/.repog/config.json
 */
export interface RepoGConfig {
  githubToken: string | null;
  geminiApiKey: string | null;
  dbPath: string;
}

/**
 * Result from a semantic search query.
 */
export interface SearchResult {
  repo: Repo;
  chunkContent: string;
  similarityScore: number;
}

/**
 * Result from a recommendation query, includes LLM-generated reasoning.
 */
export interface RecommendResult {
  repo: Repo;
  chunkContent: string;
  similarityScore: number;
  reason: string;
  confidence: 'high' | 'medium' | 'low';
}

/**
 * Options for the sync module.
 */
export interface SyncOptions {
  full: boolean;
  ownedOnly: boolean;
  starredOnly: boolean;
  limit: number | null;
}

/**
 * Options for the embed module.
 */
export interface EmbedOptions {
  force: boolean;
  repo: string | null;
}

/**
 * Options for the search module.
 */
export interface SearchOptions {
  limit: number;
  lang: string | null;
  starred: boolean | null;
  owned: boolean | null;
}
