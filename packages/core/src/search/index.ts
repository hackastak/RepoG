import type { SearchOptions, SearchResult } from '../types/index.js';

// Export new vector search module
export {
  searchRepos,
  type SearchFilters,
  type SearchResult as VectorSearchResult,
  type SearchQueryResult,
} from './query.js';

/**
 * Perform a semantic search across repositories.
 *
 * @param query - The search query
 * @param options - Search options
 * @returns Array of search results sorted by similarity
 */
export async function search(_query: string, _options: SearchOptions): Promise<SearchResult[]> {
  throw new Error('Not implemented');
}

/**
 * Search within a specific repository.
 *
 * @param query - The search query
 * @param repoFullName - Repository full name (owner/repo)
 * @param limit - Maximum results to return
 * @returns Array of search results
 */
export async function searchInRepo(
  _query: string,
  _repoFullName: string,
  _limit: number
): Promise<SearchResult[]> {
  throw new Error('Not implemented');
}

/**
 * Find similar repositories to the given query.
 *
 * @param query - The search query
 * @param limit - Maximum results to return
 * @returns Array of repositories sorted by relevance
 */
export async function findSimilarRepos(_query: string, _limit: number): Promise<SearchResult[]> {
  throw new Error('Not implemented');
}

/**
 * Log a search query for analytics.
 *
 * @param queryType - The type of query
 * @param queryText - The query text
 * @param resultsCount - Number of results returned
 * @param latencyMs - Query latency in milliseconds
 */
export function logQuery(
  _queryType: string,
  _queryText: string,
  _resultsCount: number,
  _latencyMs: number
): void {
  throw new Error('Not implemented');
}
