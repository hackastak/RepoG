import type { RecommendResult, Repo, SearchResult } from '../types/index.js';

/**
 * Ask a question and get an AI-generated answer using RAG.
 *
 * @param question - The question to ask
 * @param repoFullName - Optional repository to scope the question to
 * @returns The AI-generated answer
 */
export async function ask(_question: string, _repoFullName?: string): Promise<string> {
  throw new Error('Not implemented');
}

/**
 * Get repository recommendations based on a query.
 *
 * @param query - The recommendation query
 * @param limit - Maximum recommendations to return
 * @returns Array of recommendations with explanations
 */
export async function recommend(_query: string, _limit: number): Promise<RecommendResult[]> {
  throw new Error('Not implemented');
}

/**
 * Generate a summary of a repository.
 *
 * @param repoFullName - Repository full name (owner/repo)
 * @returns The AI-generated summary
 */
export async function summarize(_repoFullName: string): Promise<string> {
  throw new Error('Not implemented');
}

/**
 * Build a prompt context from search results.
 *
 * @param results - Search results to include in context
 * @param maxTokens - Maximum tokens for the context
 * @returns The formatted context string
 */
export function buildContext(_results: SearchResult[], _maxTokens: number): string {
  throw new Error('Not implemented');
}

/**
 * Generate a response using the Gemini API.
 *
 * @param prompt - The prompt to send
 * @param context - The context to include
 * @returns The generated response
 */
export async function generateResponse(_prompt: string, _context: string): Promise<string> {
  throw new Error('Not implemented');
}

/**
 * Get repository data for RAG context.
 *
 * @param repoFullName - Repository full name (owner/repo)
 * @returns The repository data or null
 */
export function getRepoForContext(_repoFullName: string): Repo | null {
  throw new Error('Not implemented');
}
