import { loadConfig } from '../config/config.js';
import { getDb } from '../db/index.js';
import { embedQuery } from '../gemini/embeddings.js';

/**
 * Search filters for vector similarity search.
 */
export interface SearchFilters {
  /** Filter by repos.primary_language (case-insensitive) */
  language?: string;
  /** Filter by repos.is_starred = true */
  starred?: boolean;
  /** Filter by repos.is_owned = true */
  owned?: boolean;
  /** Filter by repos.owner (case-insensitive) */
  owner?: string;
  /** Number of results to return, default 3 */
  limit?: number;
}

/**
 * A single search result with repository and chunk information.
 */
export interface SearchResult {
  repoFullName: string;
  owner: string;
  repoName: string;
  description: string | null;
  language: string | null;
  stars: number;
  isStarred: boolean;
  isOwned: boolean;
  htmlUrl: string;
  chunkType: 'metadata' | 'readme' | 'file_tree';
  content: string;
  /** Cosine similarity score, 0-1 */
  similarity: number;
}

/**
 * Result of a search query with timing metrics.
 */
export interface SearchQueryResult {
  results: SearchResult[];
  /** How many chunks were searched before deduplication */
  totalConsidered: number;
  /** Time taken to embed the query in milliseconds */
  queryEmbeddingMs: number;
  /** Time taken to run the vector search in milliseconds */
  searchMs: number;
}

/**
 * Raw row returned from the vector similarity query.
 */
interface RawSearchRow {
  full_name: string;
  owner: string;
  name: string;
  description: string | null;
  primary_language: string | null;
  stars: number;
  is_starred: number;
  is_owned: number;
  html_url: string;
  chunk_type: string;
  content: string;
  distance: number;
}

/**
 * Empty result to return on errors.
 */
function emptyResult(): SearchQueryResult {
  return {
    results: [],
    totalConsidered: 0,
    queryEmbeddingMs: 0,
    searchMs: 0,
  };
}

/**
 * Search repositories using vector similarity.
 *
 * This function:
 * 1. Embeds the query string using Gemini
 * 2. Runs a vector similarity search using sqlite-vec
 * 3. Applies optional filters (language, starred, owned, owner)
 * 4. Deduplicates results by repository, keeping highest similarity chunk
 * 5. Returns results with timing metrics
 *
 * @param query - The natural language search query
 * @param filters - Optional filters to apply
 * @returns Search results with timing metrics, never throws
 */
export async function searchRepos(
  query: string,
  filters: SearchFilters = {}
): Promise<SearchQueryResult> {
  try {
    // Load config
    const config = loadConfig();
    if (!config.geminiKey) {
      return emptyResult();
    }

    // Embed the query
    const embedStart = performance.now();
    const embedding = await embedQuery(config.geminiKey, query);
    const queryEmbeddingMs = performance.now() - embedStart;

    if (!embedding) {
      return {
        results: [],
        totalConsidered: 0,
        queryEmbeddingMs,
        searchMs: 0,
      };
    }

    // Get database connection (sqlite-vec is loaded automatically via migrate)
    const db = getDb(config.dbPath);

    // Serialize embedding as Buffer for sqlite-vec
    const embeddingBuffer = Buffer.from(new Float32Array(embedding).buffer);

    // Build dynamic WHERE clause and params
    const whereClauses: string[] = [];
    const params: (string | number | Buffer)[] = [embeddingBuffer];

    if (filters.language !== undefined) {
      whereClauses.push('LOWER(r.primary_language) = LOWER(?)');
      params.push(filters.language);
    }

    if (filters.starred === true) {
      whereClauses.push('r.is_starred = 1');
    }

    if (filters.owned === true) {
      whereClauses.push('r.is_owned = 1');
    }

    if (filters.owner !== undefined) {
      whereClauses.push('LOWER(r.owner) = LOWER(?)');
      params.push(filters.owner);
    }

    // Default limit is 3, but we fetch more to allow for deduplication
    const limit = filters.limit ?? 3;
    // Fetch extra rows to ensure we have enough after deduplication
    const fetchLimit = limit * 5;
    params.push(fetchLimit);

    // Build the SQL query
    const whereClause =
      whereClauses.length > 0 ? `AND ${whereClauses.join(' AND ')}` : '';

    const sql = `
      SELECT
        r.full_name,
        r.owner,
        r.name,
        r.description,
        r.primary_language,
        r.stars,
        r.is_starred,
        r.is_owned,
        r.url as html_url,
        c.chunk_type,
        c.content,
        vec_distance_cosine(ce.embedding, ?) as distance
      FROM chunk_embeddings ce
      JOIN chunks c ON c.id = ce.chunk_id
      JOIN repos r ON r.id = c.repo_id
      WHERE 1=1
        ${whereClause}
      ORDER BY distance ASC
      LIMIT ?
    `;

    // Execute the search
    const searchStart = performance.now();
    const rows = db.prepare(sql).all(...params) as RawSearchRow[];
    const searchMs = performance.now() - searchStart;

    const totalConsidered = rows.length;

    // Deduplicate by full_name, keeping highest similarity (lowest distance)
    const seenRepos = new Map<string, SearchResult>();
    for (const row of rows) {
      if (!seenRepos.has(row.full_name)) {
        seenRepos.set(row.full_name, {
          repoFullName: row.full_name,
          owner: row.owner,
          repoName: row.name,
          description: row.description,
          language: row.primary_language,
          stars: row.stars,
          isStarred: row.is_starred === 1,
          isOwned: row.is_owned === 1,
          htmlUrl: row.html_url,
          chunkType: row.chunk_type as 'metadata' | 'readme' | 'file_tree',
          content: row.content,
          similarity: 1 - row.distance,
        });
      }
    }

    // Convert to array and limit to requested count
    const results = Array.from(seenRepos.values()).slice(0, limit);

    return {
      results,
      totalConsidered,
      queryEmbeddingMs,
      searchMs,
    };
  } catch {
    // Never throw - return empty results on any error
    return emptyResult();
  }
}
