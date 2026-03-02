import type { RepoGConfig as TypesRepoGConfig } from '../types/index.js';
import {
  loadConfig,
  saveConfig as configSaveConfig,
  clearConfig,
  isConfigured as configIsConfigured,
} from '../config/config.js';

/**
 * Get the current RepoG configuration.
 * @deprecated Use loadConfig from config/config.js instead
 * @returns The current configuration
 */
export function getConfig(): TypesRepoGConfig {
  const config = loadConfig();
  return {
    githubToken: config.githubPat,
    geminiApiKey: config.geminiKey,
    dbPath: config.dbPath,
  };
}

/**
 * Save RepoG configuration.
 * @deprecated Use saveConfig from config/config.js instead
 * @param config - The configuration to save
 */
export function saveConfig(config: Partial<TypesRepoGConfig>): void {
  configSaveConfig({
    githubPat: config.githubToken ?? undefined,
    geminiKey: config.geminiApiKey ?? undefined,
    dbPath: config.dbPath,
  });
}

/**
 * Get the GitHub Personal Access Token.
 *
 * @returns The decrypted GitHub token or null if not set
 */
export function getGitHubToken(): string | null {
  return loadConfig().githubPat;
}

/**
 * Set the GitHub Personal Access Token.
 * The token is encrypted before storage.
 *
 * @param token - The GitHub PAT to store
 */
export function setGitHubToken(token: string): void {
  configSaveConfig({ githubPat: token });
}

/**
 * Get the Gemini API Key.
 *
 * @returns The decrypted Gemini API key or null if not set
 */
export function getGeminiApiKey(): string | null {
  return loadConfig().geminiKey;
}

/**
 * Set the Gemini API Key.
 * The key is encrypted before storage.
 *
 * @param apiKey - The Gemini API key to store
 */
export function setGeminiApiKey(apiKey: string): void {
  configSaveConfig({ geminiKey: apiKey });
}

/**
 * Check if RepoG is configured with required credentials.
 *
 * @returns True if both GitHub token and Gemini API key are set
 */
export function isConfigured(): boolean {
  return configIsConfigured();
}

/**
 * Clear all stored credentials.
 */
export function clearCredentials(): void {
  clearConfig();
}
