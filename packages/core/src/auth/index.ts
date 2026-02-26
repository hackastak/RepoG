import type { RepoGConfig } from '../types/index.js';

/**
 * Get the current RepoG configuration.
 *
 * @returns The current configuration
 */
export function getConfig(): RepoGConfig {
  throw new Error('Not implemented');
}

/**
 * Save RepoG configuration.
 *
 * @param config - The configuration to save
 */
export function saveConfig(_config: Partial<RepoGConfig>): void {
  throw new Error('Not implemented');
}

/**
 * Get the GitHub Personal Access Token.
 *
 * @returns The decrypted GitHub token or null if not set
 */
export function getGitHubToken(): string | null {
  throw new Error('Not implemented');
}

/**
 * Set the GitHub Personal Access Token.
 * The token is encrypted before storage.
 *
 * @param token - The GitHub PAT to store
 */
export function setGitHubToken(_token: string): void {
  throw new Error('Not implemented');
}

/**
 * Get the Gemini API Key.
 *
 * @returns The decrypted Gemini API key or null if not set
 */
export function getGeminiApiKey(): string | null {
  throw new Error('Not implemented');
}

/**
 * Set the Gemini API Key.
 * The key is encrypted before storage.
 *
 * @param apiKey - The Gemini API key to store
 */
export function setGeminiApiKey(_apiKey: string): void {
  throw new Error('Not implemented');
}

/**
 * Check if RepoG is configured with required credentials.
 *
 * @returns True if both GitHub token and Gemini API key are set
 */
export function isConfigured(): boolean {
  throw new Error('Not implemented');
}

/**
 * Clear all stored credentials.
 */
export function clearCredentials(): void {
  throw new Error('Not implemented');
}
