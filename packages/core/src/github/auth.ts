import { Octokit } from 'octokit';

/**
 * Result of GitHub token validation.
 */
export interface GitHubAuthResult {
  /** Whether the token is valid and has required scopes */
  valid: boolean;
  /** GitHub username if valid */
  login?: string;
  /** Scopes granted to the token */
  scopes?: string[];
  /** Error message if validation failed */
  error?: string;
}

/**
 * Required OAuth scopes for RepoG to function properly.
 * - repo: Full control of private repositories (needed to read private repo content)
 */
const REQUIRED_SCOPES = ['repo'];

/**
 * Validate a GitHub Personal Access Token.
 *
 * Checks that the token is valid and has the required 'repo' scope.
 * Does not throw - returns structured result.
 *
 * @param token - The GitHub PAT to validate
 * @returns Validation result with login info if successful
 */
export async function validateGitHubToken(token: string): Promise<GitHubAuthResult> {
  if (!token || token.trim() === '') {
    return {
      valid: false,
      error: 'Token is empty',
    };
  }

  try {
    const octokit = new Octokit({ auth: token });

    // Make authenticated request to get current user
    const response = await octokit.rest.users.getAuthenticated();

    // Get scopes from response headers
    const scopeHeader = response.headers['x-oauth-scopes'];
    const scopes = scopeHeader
      ? scopeHeader.split(',').map((s: string) => s.trim())
      : [];

    // Check for required scopes
    const hasRepoScope = scopes.some(
      (scope: string) => scope === 'repo' || scope === 'public_repo'
    );

    if (!hasRepoScope) {
      return {
        valid: false,
        login: response.data.login,
        scopes,
        error: `Token is missing required 'repo' scope. Current scopes: ${scopes.join(', ') || 'none'}`,
      };
    }

    return {
      valid: true,
      login: response.data.login,
      scopes,
    };
  } catch (error) {
    // Handle specific error types
    if (error instanceof Error) {
      // Check for 401 Unauthorized
      if ('status' in error && (error as { status: number }).status === 401) {
        return {
          valid: false,
          error: 'Invalid token - authentication failed',
        };
      }

      // Network errors or other issues
      return {
        valid: false,
        error: `Failed to validate token: ${error.message}`,
      };
    }

    return {
      valid: false,
      error: 'Unknown error validating token',
    };
  }
}

/**
 * Check if a token has a specific scope.
 *
 * @param token - The GitHub PAT to check
 * @param scope - The scope to check for
 * @returns True if the token has the scope
 */
export async function hasScope(token: string, scope: string): Promise<boolean> {
  const result = await validateGitHubToken(token);
  if (!result.valid || !result.scopes) {
    return false;
  }
  return result.scopes.includes(scope);
}

/**
 * Get the list of required scopes for RepoG.
 *
 * @returns Array of required OAuth scope names
 */
export function getRequiredScopes(): string[] {
  return [...REQUIRED_SCOPES];
}
