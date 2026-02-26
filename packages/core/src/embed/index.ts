import type { Chunk, EmbedOptions, Repo } from '../types/index.js';

/**
 * Generate embeddings for repositories.
 *
 * @param options - Embed options
 * @returns Number of chunks embedded
 */
export async function embedRepos(_options: EmbedOptions): Promise<number> {
  throw new Error('Not implemented');
}

/**
 * Generate embeddings for a single repository.
 *
 * @param repoId - The repository ID
 * @param force - Whether to re-embed existing chunks
 * @returns Number of chunks embedded
 */
export async function embedRepo(_repoId: number, _force: boolean): Promise<number> {
  throw new Error('Not implemented');
}

/**
 * Chunk repository content into smaller pieces.
 *
 * @param repo - The repository to chunk
 * @returns Array of chunks
 */
export function chunkRepo(_repo: Repo): Chunk[] {
  throw new Error('Not implemented');
}

/**
 * Generate embeddings for text content.
 *
 * @param text - The text to embed
 * @returns The embedding vector
 */
export async function generateEmbedding(_text: string): Promise<number[]> {
  throw new Error('Not implemented');
}

/**
 * Get the count of repositories pending embedding.
 *
 * @returns Number of repositories without embeddings
 */
export function getPendingEmbedCount(): number {
  throw new Error('Not implemented');
}

/**
 * Delete embeddings for a repository.
 *
 * @param repoId - The repository ID
 */
export function deleteEmbeddings(_repoId: number): void {
  throw new Error('Not implemented');
}
