import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { validateGitHubToken, hasScope, getRequiredScopes } from './auth.js';

// Mock the Octokit module
vi.mock('octokit', () => {
  return {
    Octokit: vi.fn(),
  };
});

// Import after mock setup
import { Octokit } from 'octokit';

const mockOctokit = vi.mocked(Octokit);

describe('validateGitHubToken', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('valid tokens', () => {
    it('returns valid: true with correct login when API returns 200 with repo scope', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'testuser', id: 12345 },
              headers: { 'x-oauth-scopes': 'repo, read:user' },
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_validtoken123');

      expect(result.valid).toBe(true);
      expect(result.login).toBe('testuser');
      expect(result.scopes).toContain('repo');
      expect(result.error).toBeUndefined();
    });

    it('accepts public_repo scope as valid', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'publicuser', id: 67890 },
              headers: { 'x-oauth-scopes': 'public_repo, read:user' },
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_publictoken123');

      expect(result.valid).toBe(true);
      expect(result.login).toBe('publicuser');
    });
  });

  describe('invalid tokens', () => {
    it('returns valid: false when API returns 401', async () => {
      const error = new Error('Bad credentials');
      (error as Error & { status: number }).status = 401;

      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(error),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_invalidtoken');

      expect(result.valid).toBe(false);
      expect(result.error).toContain('Invalid token');
      expect(result.login).toBeUndefined();
    });

    it('returns valid: false when token is empty', async () => {
      const result = await validateGitHubToken('');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('Token is empty');
    });

    it('returns valid: false when token is whitespace only', async () => {
      const result = await validateGitHubToken('   ');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('Token is empty');
    });

    it('returns valid: false when token lacks required scope', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'limiteduser', id: 11111 },
              headers: { 'x-oauth-scopes': 'read:user, user:email' },
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_limitedscopetoken');

      expect(result.valid).toBe(false);
      expect(result.login).toBe('limiteduser');
      expect(result.error).toContain("missing required 'repo' scope");
    });
  });

  describe('network errors', () => {
    it('returns valid: false on network error and does not throw', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(new Error('Network connection failed')),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_sometoken');

      expect(result.valid).toBe(false);
      expect(result.error).toBeDefined();
      expect(result.error).toContain('Failed to validate token');
    });

    it('returns valid: false on timeout and does not throw', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(new Error('ETIMEDOUT')),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_timeouttoken');

      expect(result.valid).toBe(false);
      expect(result.error).toBeDefined();
    });
  });

  describe('edge cases', () => {
    it('handles missing x-oauth-scopes header', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'noscopeuser', id: 22222 },
              headers: {},
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_noscopestoken');

      expect(result.valid).toBe(false);
      expect(result.login).toBe('noscopeuser');
      expect(result.scopes).toEqual([]);
      expect(result.error).toContain("missing required 'repo' scope");
    });
  });
});

describe('hasScope', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns true when token has the specified scope', async () => {
    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockResolvedValue({
            data: { login: 'user', id: 1 },
            headers: { 'x-oauth-scopes': 'repo, read:user, admin:org' },
          }),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('ghp_token', 'admin:org');

    expect(result).toBe(true);
  });

  it('returns false when token lacks the specified scope', async () => {
    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockResolvedValue({
            data: { login: 'user', id: 1 },
            headers: { 'x-oauth-scopes': 'repo, read:user' },
          }),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('ghp_token', 'admin:org');

    expect(result).toBe(false);
  });

  it('returns false for invalid token', async () => {
    const error = new Error('Bad credentials');
    (error as Error & { status: number }).status = 401;

    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockRejectedValue(error),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('ghp_invalid', 'repo');

    expect(result).toBe(false);
  });
});

describe('getRequiredScopes', () => {
  it('returns array containing repo scope', () => {
    const scopes = getRequiredScopes();

    expect(Array.isArray(scopes)).toBe(true);
    expect(scopes).toContain('repo');
  });

  it('returns a new array each time (not mutable)', () => {
    const scopes1 = getRequiredScopes();
    const scopes2 = getRequiredScopes();

    expect(scopes1).not.toBe(scopes2);
    expect(scopes1).toEqual(scopes2);
  });
});
