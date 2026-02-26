import type { Repo, SyncOptions, SyncState } from '../types/index.js';

/**
 * Sync repositories from GitHub.
 *
 * @param options - Sync options
 * @returns Array of synced repositories
 */
export async function syncRepos(_options: SyncOptions): Promise<Repo[]> {
  throw new Error('Not implemented');
}

/**
 * Sync a single repository by full name.
 *
 * @param fullName - Repository full name (owner/repo)
 * @returns The synced repository
 */
export async function syncRepo(_fullName: string): Promise<Repo> {
  throw new Error('Not implemented');
}

/**
 * Get the current sync state.
 *
 * @returns The most recent sync state or null
 */
export function getSyncState(): SyncState | null {
  throw new Error('Not implemented');
}

/**
 * Fetch and update the README for a repository.
 *
 * @param repoId - The repository ID
 * @returns The updated repository
 */
export async function fetchReadme(_repoId: number): Promise<Repo> {
  throw new Error('Not implemented');
}

/**
 * Fetch and update the file tree for a repository.
 *
 * @param repoId - The repository ID
 * @returns The updated repository
 */
export async function fetchFileTree(_repoId: number): Promise<Repo> {
  throw new Error('Not implemented');
}

/**
 * Resume an interrupted sync operation.
 *
 * @param syncStateId - The sync state ID to resume
 * @returns Array of synced repositories
 */
export async function resumeSync(_syncStateId: number): Promise<Repo[]> {
  throw new Error('Not implemented');
}
