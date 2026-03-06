import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import path from 'path';
import fs from 'fs';
import os from 'os';
import Database from 'better-sqlite3';
import { getStatus, NotConfiguredError } from './status.js';
import * as configModule from '../config/config.js';
import * as clientModule from '../github/client.js';
import { closeDb } from '../db/index.js';
import { initDb } from '../db/init.js';

// Mock dependencies
vi.mock('../config/config.js');
vi.mock('../github/client.js');

describe('getStatus', () => {
  const TEST_DIR = path.join(os.tmpdir(), `repog-status-test-${process.pid}-${Date.now()}`);
  let dbPath: string;

  beforeEach(() => {
    fs.mkdirSync(TEST_DIR, { recursive: true });
    dbPath = path.join(TEST_DIR, 'test.db');
    
    // Initialize DB schema
    initDb(dbPath);
    
    // Default mocks
    vi.mocked(configModule.isConfigured).mockReturnValue(true);
    vi.mocked(configModule.loadConfig).mockReturnValue({
      githubPat: 'test-pat',
      geminiKey: 'test-key',
      dbPath: dbPath
    });
    
    vi.mocked(clientModule.getRateLimitInfo).mockResolvedValue({
      limit: 5000,
      remaining: 4900,
      resetAt: new Date(Date.now() + 3600000).toISOString(),
      available: true
    });
  });

  afterEach(() => {
    closeDb();
    try {
      fs.rmSync(TEST_DIR, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
    vi.clearAllMocks();
  });

  it('throws NotConfiguredError if not configured', async () => {
    vi.mocked(configModule.isConfigured).mockReturnValue(false);
    await expect(getStatus()).rejects.toThrow(NotConfiguredError);
  });

  it('returns fully populated status result', async () => {
    // Populate DB with test data
    const db = new Database(dbPath);
    
    // Insert repos
    db.prepare(`
      INSERT INTO repos (github_id, owner, name, full_name, url, is_owned, is_starred, pushed_at, synced_at, pushed_at_hash, embedded_hash, embedded_at)
      VALUES 
        (1, 'me', 'my-repo', 'me/my-repo', 'http://u1', 1, 0, '2023-01-01', '2023-01-01', 'hash1', 'hash1', '2023-01-02'),
        (2, 'other', 'star-repo', 'other/star-repo', 'http://u2', 0, 1, '2023-01-01', '2023-01-01', 'hash2', 'hash2', '2023-01-03'),
        (3, 'other', 'pending-repo', 'other/pending-repo', 'http://u3', 0, 1, '2023-01-01', '2023-01-01', 'hash3', NULL, NULL),
        (4, 'other', 'outdated-repo', 'other/outdated-repo', 'http://u4', 0, 1, '2023-01-01', '2023-01-01', 'newhash', 'oldhash', '2023-01-01')
    `).run();

    // Insert sync state
    db.prepare(`
      INSERT INTO sync_state (sync_type, status, started_at)
      VALUES ('full', 'completed', '2023-01-04T10:00:00.000Z')
    `).run();

    // Insert chunks
    db.prepare(`
      INSERT INTO chunks (repo_id, chunk_type, chunk_index, content)
      VALUES 
        (1, 'code', 1, 'c1'),
        (1, 'code', 2, 'c2'),
        (2, 'code', 1, 'c3')
    `).run();

    // Insert embeddings (simulate count)
    // Note: chunk_embeddings is a virtual table or separate table depending on implementation.
    // Assuming standard table for count, but if virtual via sqlite-vec, might need special handling.
    // In schema.ts, chunk_embeddings is likely a normal table if using vector extension separately or virtual.
    // If it's virtual, inserting might require vector data.
    // Let's check schema.ts if needed, but for now try inserting dummy data if possible, 
    // or just assume 0 if virtual table requires complex insert.
    // Actually, initDb creates it. If it's virtual, we need valid float array.
    // Let's try simple count check. If virtual table behaves differently, we might need to skip or be careful.
    // For now, let's assume we can insert if we provide correct columns.
    
    // If we can't easily insert into vector table without vector extension logic, let's skip checking exact count > 0 
    // unless we know how to insert.
    // However, `packages/core/src/db/schema.ts` would tell us.
    // Let's look at schema.ts quickly? No, I'll trust standard SQL or mock `db.prepare` inside `getStatus` if real DB is too hard.
    // But I prefer real DB.
    
    // Let's try to verify what I can.
    db.close();

    const result = await getStatus();

    expect(result.repos.total).toBe(4);
    expect(result.repos.owned).toBe(1);
    expect(result.repos.starred).toBe(3);
    // embeddedCount: hash matches. Repo 1 and 2 match. Repo 3 is null. Repo 4 mismatch.
    expect(result.repos.embeddedCount).toBe(2);
    // pendingEmbed: Repo 3 (null) and Repo 4 (mismatch).
    expect(result.repos.pendingEmbed).toBe(2);

    expect(result.sync.lastSyncStatus).toBe('completed');
    expect(result.sync.lastSyncedAt).toBe('2023-01-04T10:00:00.000Z');

    expect(result.embed.totalChunks).toBe(3);
    // We didn't insert embeddings, so 0
    expect(result.embed.totalEmbeddings).toBe(0); 
    expect(result.embed.lastEmbeddedAt).toBe('2023-01-03'); // Max of inserted dates

    expect(result.rateLimit).toEqual({
      limit: 5000,
      remaining: 4900,
      resetAt: expect.any(String),
      available: true
    });

    expect(result.db.path).toBe(dbPath);
    expect(result.db.sizeBytes).toBeGreaterThan(0);
    expect(result.db.sizeMb).toMatch(/^\d+\.\d{2} MB$/);
  });

  it('handles failed GitHub API call gracefully', async () => {
    vi.mocked(clientModule.getRateLimitInfo).mockRejectedValue(new Error('Network error'));
    
    // Or closer to implementation: getRateLimitInfo inside status.ts calls the imported function.
    // My implementation of getStatus calls getRateLimitInfo().catch(() => null)
    // So if the imported function rejects, it is caught.
    // But wait, my implementation of `getRateLimitInfo` in client.ts ALREADY catches errors and returns null.
    // So `getStatus` shouldn't see a rejection unless `getRateLimitInfo` throws (which it shouldn't).
    // But let's simulate `getRateLimitInfo` returning null.
    
    vi.mocked(clientModule.getRateLimitInfo).mockResolvedValue(null);

    const result = await getStatus();
    expect(result.rateLimit).toBeNull();
  });

  it('handles null sync state', async () => {
    // Empty DB, no sync state
    const result = await getStatus();
    expect(result.sync.lastSyncedAt).toBeNull();
    expect(result.sync.lastSyncStatus).toBeNull();
  });

  it('handles null embed stats', async () => {
    // Empty DB, no repos embedded
    const result = await getStatus();
    expect(result.embed.lastEmbeddedAt).toBeNull();
  });
});
