import type Database from 'better-sqlite3';
import { ALL_SCHEMA_STATEMENTS, VECTOR_SCHEMA_STATEMENTS } from './schema.js';

/**
 * Run database schema migrations.
 * Creates all tables and indexes if they don't exist.
 *
 * @param db - The better-sqlite3 database instance
 */
export function migrate(db: Database.Database): void {
  // Run all standard schema statements in a transaction
  db.transaction(() => {
    for (const statement of ALL_SCHEMA_STATEMENTS) {
      db.exec(statement);
    }
  })();

  // Try to create vector table (may fail if sqlite-vec extension not loaded)
  try {
    for (const statement of VECTOR_SCHEMA_STATEMENTS) {
      db.exec(statement);
    }
  } catch {
    // Vector extension not available - this is expected on first run
    // The extension will be loaded when needed
    console.warn('sqlite-vec extension not loaded, vector table creation deferred');
  }

  console.log('Database schema ready');
}
