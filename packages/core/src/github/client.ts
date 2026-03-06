import { Octokit } from 'octokit';
import { loadConfig } from '../config/config.js';

export interface RateLimitStats {
  limit: number;
  remaining: number;
  resetAt: string;   // ISO datetime string
  available: boolean; // true if remaining > 0
}

/**
 * Get the current GitHub API rate limit status.
 *
 * @returns Rate limit statistics or null if the request fails
 */
export async function getRateLimitInfo(): Promise<RateLimitStats | null> {
  const config = loadConfig();
  
  // If no token is configured, we can't check rate limits accurately for authenticated user
  // (Unauthenticated requests have very low limits)
  if (!config.githubPat) {
    return null;
  }

  try {
    const octokit = new Octokit({ auth: config.githubPat });
    const { data } = await octokit.rest.rateLimit.get();
    
    // Use core rate limit which applies to most API calls
    const core = data.resources.core;
    
    return {
      limit: core.limit,
      remaining: core.remaining,
      resetAt: new Date(core.reset * 1000).toISOString(),
      available: core.remaining > 0
    };
  } catch {
    // If request fails (network error, auth error, etc.), return null
    // The caller is expected to handle this gracefully
    return null;
  }
}
