import fs from 'fs';
import { getDb } from '../db/index.js';
import { loadConfig, isConfigured } from '../config/config.js';
import { getRateLimitInfo, type RateLimitStats } from '../github/client.js';

/**
 * Error thrown when RepoG is not configured.
 */
export class NotConfiguredError extends Error {
  constructor() {
    super('RepoG is not configured. Run `repog init` first.');
    this.name = 'NotConfiguredError';
  }
}

export interface RepoStats {
  total: number;
  owned: number;
  starred: number;
  embeddedCount: number;   // repos where embedded_hash = pushed_at_hash
  pendingEmbed: number;    // repos where embedded_hash IS NULL or != pushed_at_hash
}

export interface SyncStats {
  lastSyncedAt: string | null;   // ISO datetime string of most recent completed sync
  lastSyncStatus: 'completed' | 'failed' | 'in_progress' | null;
}

export interface EmbedStats {
  lastEmbeddedAt: string | null;  // ISO datetime string from most recently embedded repo
  totalChunks: number;
  totalEmbeddings: number;        // count of rows in chunk_embeddings
}

export { RateLimitStats };

export interface DbStats {
  path: string;
  sizeBytes: number;
  sizeMb: string;   // formatted to 2 decimal places e.g. "4.21 MB"
}

export interface StatusResult {
  repos: RepoStats;
  sync: SyncStats;
  embed: EmbedStats;
  rateLimit: RateLimitStats | null;  // null if GitHub API call fails
  db: DbStats;
  generatedAt: string;  // ISO datetime string of when status was generated
}

/**
 * Get the current status of the RepoG system.
 *
 * @returns comprehensive status result
 * @throws NotConfiguredError if the system is not configured
 */
export async function getStatus(): Promise<StatusResult> {
  if (!isConfigured()) {
    throw new NotConfiguredError();
  }

  const config = loadConfig();
  const dbPath = config.dbPath;
  const db = getDb(dbPath);

  // Run independent tasks in parallel
  const [
    repoStats,
    syncState,
    chunkCount,
    embeddingCount,
    lastEmbedded,
    rateLimitInfo,
    dbStats
  ] = await Promise.all([
    // Repo stats
    (async () => {
      const row = db.prepare(`
        SELECT
          COUNT(*) as total,
          SUM(CASE WHEN is_owned = 1 THEN 1 ELSE 0 END) as owned,
          SUM(CASE WHEN is_starred = 1 THEN 1 ELSE 0 END) as starred,
          SUM(CASE WHEN embedded_hash IS NOT NULL AND embedded_hash = pushed_at_hash THEN 1 ELSE 0 END) as embeddedCount,
          SUM(CASE WHEN embedded_hash IS NULL OR embedded_hash != pushed_at_hash THEN 1 ELSE 0 END) as pendingEmbed
        FROM repos
      `).get() as {
        total: number;
        owned: number | null;
        starred: number | null;
        embeddedCount: number | null;
        pendingEmbed: number | null;
      };
      return {
        total: row.total,
        owned: row.owned || 0,
        starred: row.starred || 0,
        embeddedCount: row.embeddedCount || 0,
        pendingEmbed: row.pendingEmbed || 0
      };
    })(),

    // Sync stats
    (async () => {
      const row = db.prepare(`
        SELECT status, started_at
        FROM sync_state
        ORDER BY started_at DESC
        LIMIT 1
      `).get() as { status: string; started_at: string } | undefined;
      return row;
    })(),

    // Embed stats - chunks
    (async () => {
      const row = db.prepare('SELECT COUNT(*) as totalChunks FROM chunks').get() as { totalChunks: number };
      return row.totalChunks;
    })(),

    // Embed stats - embeddings
    (async () => {
      const row = db.prepare('SELECT COUNT(*) as totalEmbeddings FROM chunk_embeddings').get() as { totalEmbeddings: number };
      return row.totalEmbeddings;
    })(),

    // Embed stats - last embedded
    (async () => {
      const row = db.prepare('SELECT MAX(embedded_at) as lastEmbeddedAt FROM repos WHERE embedded_at IS NOT NULL').get() as { lastEmbeddedAt: string | null };
      return row.lastEmbeddedAt;
    })(),

    // GitHub Rate Limit
    getRateLimitInfo().catch(() => null),

    // DB Stats
    (async () => {
      try {
        const stats = fs.statSync(dbPath);
        const sizeBytes = stats.size;
        const sizeMb = (sizeBytes / (1024 * 1024)).toFixed(2) + ' MB';
        return { path: dbPath, sizeBytes, sizeMb };
      } catch {
        // Fallback if DB file can't be stat'd (unlikely if getDb worked)
        return { path: dbPath, sizeBytes: 0, sizeMb: '0.00 MB' };
      }
    })()
  ]);

  return {
    repos: repoStats,
    sync: {
      lastSyncedAt: syncState?.started_at || null,
      lastSyncStatus: (syncState?.status as SyncStats['lastSyncStatus']) || null
    },
    embed: {
      lastEmbeddedAt: lastEmbedded,
      totalChunks: chunkCount,
      totalEmbeddings: embeddingCount
    },
    rateLimit: rateLimitInfo,
    db: dbStats,
    generatedAt: new Date().toISOString()
  };
}
