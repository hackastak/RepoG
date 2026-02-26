/**
 * Database schema definitions for RepoG.
 * These are string constants to be executed during migration.
 */

export const CREATE_REPOS_TABLE = `
CREATE TABLE IF NOT EXISTS repos (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  github_id        INTEGER NOT NULL UNIQUE,
  owner            TEXT NOT NULL,
  name             TEXT NOT NULL,
  full_name        TEXT NOT NULL UNIQUE,
  description      TEXT,
  url              TEXT NOT NULL,
  homepage         TEXT,
  primary_language TEXT,
  topics           TEXT,
  stars            INTEGER DEFAULT 0,
  is_fork          INTEGER DEFAULT 0,
  is_private       INTEGER DEFAULT 0,
  is_starred       INTEGER DEFAULT 0,
  is_owned         INTEGER DEFAULT 0,
  readme           TEXT,
  file_tree        TEXT,
  pushed_at        TEXT NOT NULL,
  synced_at        TEXT NOT NULL,
  embedded_at      TEXT,
  pushed_at_hash   TEXT,
  created_at       TEXT NOT NULL DEFAULT (datetime('now'))
);
`;

export const CREATE_CHUNKS_TABLE = `
CREATE TABLE IF NOT EXISTS chunks (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id         INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  chunk_type      TEXT NOT NULL,
  chunk_index     INTEGER NOT NULL,
  content         TEXT NOT NULL,
  token_estimate  INTEGER,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(repo_id, chunk_type, chunk_index)
);
`;

export const CREATE_EMBEDDINGS_TABLE = `
CREATE VIRTUAL TABLE IF NOT EXISTS embeddings USING vec0(
  chunk_id  INTEGER PRIMARY KEY,
  embedding FLOAT[768]
);
`;

export const CREATE_SYNC_STATE_TABLE = `
CREATE TABLE IF NOT EXISTS sync_state (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_type        TEXT NOT NULL,
  status           TEXT NOT NULL,
  total_repos      INTEGER DEFAULT 0,
  processed_repos  INTEGER DEFAULT 0,
  last_cursor      TEXT,
  started_at       TEXT NOT NULL DEFAULT (datetime('now')),
  completed_at     TEXT,
  error            TEXT
);
`;

export const CREATE_QUERY_LOG_TABLE = `
CREATE TABLE IF NOT EXISTS query_log (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  query_type      TEXT NOT NULL,
  query_text      TEXT NOT NULL,
  repo_full_name  TEXT,
  results_count   INTEGER,
  latency_ms      INTEGER,
  created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
`;

// Index definitions
export const CREATE_REPOS_GITHUB_ID_INDEX = `
CREATE INDEX IF NOT EXISTS idx_repos_github_id ON repos(github_id);
`;

export const CREATE_REPOS_FULL_NAME_INDEX = `
CREATE INDEX IF NOT EXISTS idx_repos_full_name ON repos(full_name);
`;

export const CREATE_REPOS_OWNER_INDEX = `
CREATE INDEX IF NOT EXISTS idx_repos_owner ON repos(owner);
`;

export const CREATE_REPOS_LANGUAGE_INDEX = `
CREATE INDEX IF NOT EXISTS idx_repos_primary_language ON repos(primary_language);
`;

export const CREATE_REPOS_STARRED_INDEX = `
CREATE INDEX IF NOT EXISTS idx_repos_is_starred ON repos(is_starred);
`;

export const CREATE_REPOS_OWNED_INDEX = `
CREATE INDEX IF NOT EXISTS idx_repos_is_owned ON repos(is_owned);
`;

export const CREATE_CHUNKS_REPO_ID_INDEX = `
CREATE INDEX IF NOT EXISTS idx_chunks_repo_id ON chunks(repo_id);
`;

export const CREATE_CHUNKS_TYPE_INDEX = `
CREATE INDEX IF NOT EXISTS idx_chunks_chunk_type ON chunks(chunk_type);
`;

export const CREATE_SYNC_STATE_STATUS_INDEX = `
CREATE INDEX IF NOT EXISTS idx_sync_state_status ON sync_state(status);
`;

export const CREATE_QUERY_LOG_TYPE_INDEX = `
CREATE INDEX IF NOT EXISTS idx_query_log_query_type ON query_log(query_type);
`;

export const CREATE_QUERY_LOG_CREATED_AT_INDEX = `
CREATE INDEX IF NOT EXISTS idx_query_log_created_at ON query_log(created_at);
`;

/**
 * All schema statements in order of execution.
 */
export const ALL_SCHEMA_STATEMENTS = [
  CREATE_REPOS_TABLE,
  CREATE_CHUNKS_TABLE,
  CREATE_SYNC_STATE_TABLE,
  CREATE_QUERY_LOG_TABLE,
  // Indexes for repos
  CREATE_REPOS_GITHUB_ID_INDEX,
  CREATE_REPOS_FULL_NAME_INDEX,
  CREATE_REPOS_OWNER_INDEX,
  CREATE_REPOS_LANGUAGE_INDEX,
  CREATE_REPOS_STARRED_INDEX,
  CREATE_REPOS_OWNED_INDEX,
  // Indexes for chunks
  CREATE_CHUNKS_REPO_ID_INDEX,
  CREATE_CHUNKS_TYPE_INDEX,
  // Indexes for sync_state
  CREATE_SYNC_STATE_STATUS_INDEX,
  // Indexes for query_log
  CREATE_QUERY_LOG_TYPE_INDEX,
  CREATE_QUERY_LOG_CREATED_AT_INDEX,
];

/**
 * Vector table schema - requires sqlite-vec extension.
 * This is separate because it may fail if the extension isn't loaded.
 */
export const VECTOR_SCHEMA_STATEMENTS = [CREATE_EMBEDDINGS_TABLE];
