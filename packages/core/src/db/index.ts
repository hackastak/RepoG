import Database from 'better-sqlite3';
import { migrate } from './migrate.js';

let dbInstance: Database.Database | null = null;
let currentDbPath: string | null = null;

/**
 * Get or create a database connection singleton.
 * Runs migrations on first connection.
 *
 * @param dbPath - Path to the SQLite database file
 * @returns The better-sqlite3 database instance
 */
export function getDb(dbPath: string): Database.Database {
  // Return existing instance if same path
  if (dbInstance && currentDbPath === dbPath) {
    return dbInstance;
  }

  // Close existing connection if different path
  if (dbInstance && currentDbPath !== dbPath) {
    dbInstance.close();
    dbInstance = null;
    currentDbPath = null;
  }

  // Create new connection
  dbInstance = new Database(dbPath);
  currentDbPath = dbPath;

  // Enable foreign keys
  dbInstance.pragma('foreign_keys = ON');

  // Run migrations
  migrate(dbInstance);

  return dbInstance;
}

/**
 * Close the database connection.
 */
export function closeDb(): void {
  if (dbInstance) {
    dbInstance.close();
    dbInstance = null;
    currentDbPath = null;
  }
}

export { migrate } from './migrate.js';
export * from './schema.js';
