import Conf from 'conf';
import CryptoJS from 'crypto-js';
import os from 'os';
import path from 'path';

/**
 * Configuration schema stored by conf.
 * Credentials are stored encrypted.
 */
interface StoredConfig {
  githubPat?: string;
  geminiKey?: string;
  dbPath?: string;
}

/**
 * Public configuration interface with decrypted values.
 * Named differently from types/index.ts RepoGConfig to avoid conflicts.
 */
export interface ConfigData {
  githubPat: string | null;
  geminiKey: string | null;
  dbPath: string;
}

/**
 * Result of a save operation.
 */
export interface SaveConfigResult {
  success: boolean;
  error?: string;
}

// Encryption key derived from machine-specific data
// This provides basic protection against casual inspection
const ENCRYPTION_KEY = `repog-${os.hostname()}-${os.userInfo().username}`;

// Default database path
const DEFAULT_DB_PATH = path.resolve(os.homedir(), '.repog', 'repog.db');

// Singleton conf instance
let configStore: Conf<StoredConfig> | null = null;

/**
 * Get or create the conf store instance.
 * @returns The conf store instance
 */
function getStore(): Conf<StoredConfig> {
  if (!configStore) {
    configStore = new Conf<StoredConfig>({
      projectName: 'repog',
      cwd: path.resolve(os.homedir(), '.repog'),
      configName: 'config',
    });
  }
  return configStore;
}

/**
 * Encrypt a value using AES encryption.
 * @param value - The plaintext value to encrypt
 * @returns The encrypted value as a base64 string
 */
function encrypt(value: string): string {
  return CryptoJS.AES.encrypt(value, ENCRYPTION_KEY).toString();
}

/**
 * Decrypt a value that was encrypted with our key.
 * @param encrypted - The encrypted base64 string
 * @returns The decrypted plaintext value, or null if decryption fails
 */
function decrypt(encrypted: string): string | null {
  try {
    const bytes = CryptoJS.AES.decrypt(encrypted, ENCRYPTION_KEY);
    const decrypted = bytes.toString(CryptoJS.enc.Utf8);
    return decrypted || null;
  } catch {
    return null;
  }
}

/**
 * Save configuration values.
 * Credentials (githubPat, geminiKey) are encrypted before storage.
 *
 * @param config - Partial configuration to save (merged with existing)
 * @returns Result indicating success or failure
 */
export function saveConfig(config: Partial<ConfigData>): SaveConfigResult {
  try {
    const store = getStore();

    if (config.githubPat !== undefined) {
      if (config.githubPat === null) {
        store.delete('githubPat');
      } else {
        store.set('githubPat', encrypt(config.githubPat));
      }
    }

    if (config.geminiKey !== undefined) {
      if (config.geminiKey === null) {
        store.delete('geminiKey');
      } else {
        store.set('geminiKey', encrypt(config.geminiKey));
      }
    }

    if (config.dbPath !== undefined) {
      store.set('dbPath', config.dbPath);
    }

    return { success: true };
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error saving config',
    };
  }
}

/**
 * Load the current configuration.
 * Credentials are decrypted before returning.
 *
 * @returns The current configuration with decrypted values
 */
export function loadConfig(): ConfigData {
  const store = getStore();

  const encryptedPat = store.get('githubPat');
  const encryptedKey = store.get('geminiKey');
  const storedDbPath = store.get('dbPath');

  return {
    githubPat: encryptedPat ? decrypt(encryptedPat) : null,
    geminiKey: encryptedKey ? decrypt(encryptedKey) : null,
    dbPath: storedDbPath ?? DEFAULT_DB_PATH,
  };
}

/**
 * Check if RepoG is configured with both required credentials.
 *
 * @returns True if both GitHub PAT and Gemini API key are set and decryptable
 */
export function isConfigured(): boolean {
  const config = loadConfig();
  return config.githubPat !== null && config.geminiKey !== null;
}

/**
 * Clear all stored configuration values.
 *
 * @returns Result indicating success or failure
 */
export function clearConfig(): SaveConfigResult {
  try {
    const store = getStore();
    store.clear();
    return { success: true };
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error clearing config',
    };
  }
}

/**
 * Get the default database path.
 *
 * @returns The default path to the SQLite database
 */
export function getDefaultDbPath(): string {
  return DEFAULT_DB_PATH;
}

/**
 * Reset the config store instance (for testing purposes).
 * @internal
 */
export function _resetStore(): void {
  configStore = null;
}

/**
 * Get the raw encrypted value for a key (for testing purposes).
 * @internal
 */
export function _getRawValue(key: keyof StoredConfig): string | undefined {
  return getStore().get(key);
}
