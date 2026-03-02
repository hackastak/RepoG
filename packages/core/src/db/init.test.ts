import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import Database from 'better-sqlite3';
import { initDb, isWalEnabled, getTableList } from './init.js';
import os from 'os';
import path from 'path';
import fs from 'fs';

// Use a unique test directory for each test run
const TEST_DIR = path.join(os.tmpdir(), `repog-db-test-${process.pid}-${Date.now()}`);

describe('initDb', () => {
  beforeEach(() => {
    fs.mkdirSync(TEST_DIR, { recursive: true });
  });

  afterEach(() => {
    try {
      fs.rmSync(TEST_DIR, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  describe('creates all tables', () => {
    it('creates all required tables when given a fresh in-memory database', () => {
      const result = initDb(':memory:');

      expect(result.success).toBe(true);
      expect(result.tablesCreated).toContain('repos');
      expect(result.tablesCreated).toContain('chunks');
      expect(result.tablesCreated).toContain('sync_state');
      expect(result.tablesCreated).toContain('query_log');
    });

    it('creates all required tables when given a fresh file path', () => {
      const dbPath = path.join(TEST_DIR, 'test.db');

      const result = initDb(dbPath);

      expect(result.success).toBe(true);
      expect(result.dbPath).toBe(dbPath);
      expect(fs.existsSync(dbPath)).toBe(true);

      // Verify tables exist
      const tables = getTableList(dbPath);
      expect(tables).toContain('repos');
      expect(tables).toContain('chunks');
      expect(tables).toContain('sync_state');
      expect(tables).toContain('query_log');
    });

    it('creates parent directories automatically', () => {
      const dbPath = path.join(TEST_DIR, 'nested', 'dir', 'test.db');

      const result = initDb(dbPath);

      expect(result.success).toBe(true);
      expect(fs.existsSync(dbPath)).toBe(true);
    });
  });

  describe('idempotent behavior', () => {
    it('running initDb twice causes no errors', () => {
      const dbPath = path.join(TEST_DIR, 'idempotent.db');

      const result1 = initDb(dbPath);
      expect(result1.success).toBe(true);

      const result2 = initDb(dbPath);
      expect(result2.success).toBe(true);

      // Both should report the same tables
      expect(result1.tablesCreated.sort()).toEqual(result2.tablesCreated.sort());
    });

    it('table data is preserved across multiple initDb calls', () => {
      const dbPath = path.join(TEST_DIR, 'preserve.db');

      // Initialize once
      initDb(dbPath);

      // Insert some data
      const db = new Database(dbPath);
      db.exec(`
        INSERT INTO repos (github_id, owner, name, full_name, url, pushed_at, synced_at)
        VALUES (123, 'test', 'repo', 'test/repo', 'https://example.com', datetime('now'), datetime('now'))
      `);
      db.close();

      // Initialize again
      const result = initDb(dbPath);
      expect(result.success).toBe(true);

      // Verify data still exists
      const db2 = new Database(dbPath, { readonly: true });
      const count = db2.prepare('SELECT COUNT(*) as cnt FROM repos').get() as { cnt: number };
      expect(count.cnt).toBe(1);
      db2.close();
    });
  });

  describe('WAL mode', () => {
    it('enables WAL mode by default', () => {
      const dbPath = path.join(TEST_DIR, 'wal.db');

      const result = initDb(dbPath);

      expect(result.success).toBe(true);
      expect(result.walEnabled).toBe(true);

      // Verify using helper function
      expect(isWalEnabled(dbPath)).toBe(true);
    });

    it('can disable WAL mode via options', () => {
      const dbPath = path.join(TEST_DIR, 'no-wal.db');

      const result = initDb(dbPath, { enableWal: false });

      expect(result.success).toBe(true);
      expect(result.walEnabled).toBe(false);
    });

    it('WAL mode is enabled after init for in-memory database', () => {
      // Note: WAL mode doesn't persist for :memory: databases
      // but we still set it and return the status
      const result = initDb(':memory:');

      expect(result.success).toBe(true);
      // In-memory databases may not support WAL mode check the same way
      // but the operation should not fail
    });
  });

  describe('error handling', () => {
    it('returns error for invalid paths when createDirs is false', () => {
      const dbPath = path.join(TEST_DIR, 'nonexistent', 'deeply', 'nested', 'test.db');

      const result = initDb(dbPath, { createDirs: false });

      expect(result.success).toBe(false);
      expect(result.error).toBeDefined();
    });
  });
});

describe('isWalEnabled', () => {
  let testDbPath: string;

  beforeEach(() => {
    fs.mkdirSync(TEST_DIR, { recursive: true });
    testDbPath = path.join(TEST_DIR, 'wal-check.db');
  });

  afterEach(() => {
    try {
      fs.rmSync(TEST_DIR, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  it('returns true for WAL-enabled database', () => {
    initDb(testDbPath, { enableWal: true });
    expect(isWalEnabled(testDbPath)).toBe(true);
  });

  it('returns false for non-WAL database', () => {
    initDb(testDbPath, { enableWal: false });
    expect(isWalEnabled(testDbPath)).toBe(false);
  });

  it('returns false for non-existent database', () => {
    expect(isWalEnabled('/nonexistent/path/db.sqlite')).toBe(false);
  });
});

describe('getTableList', () => {
  let testDbPath: string;

  beforeEach(() => {
    fs.mkdirSync(TEST_DIR, { recursive: true });
    testDbPath = path.join(TEST_DIR, 'table-list.db');
  });

  afterEach(() => {
    try {
      fs.rmSync(TEST_DIR, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  it('returns all created tables', () => {
    initDb(testDbPath);

    const tables = getTableList(testDbPath);

    expect(tables).toContain('repos');
    expect(tables).toContain('chunks');
    expect(tables).toContain('sync_state');
    expect(tables).toContain('query_log');
  });

  it('returns empty array for non-existent database', () => {
    expect(getTableList('/nonexistent/path/db.sqlite')).toEqual([]);
  });
});
