import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { saveConfig, loadConfig, isConfigured, clearConfig, _resetStore, _getRawValue } from './config.js';
import os from 'os';
import path from 'path';
import fs from 'fs';

// Use a unique test directory to avoid conflicts
const TEST_CONFIG_DIR = path.join(os.tmpdir(), `repog-test-${process.pid}-${Date.now()}`);

// Override conf's directory for testing
const originalHomedir = os.homedir;

describe('config', () => {
  beforeEach(() => {
    // Reset the singleton store before each test
    _resetStore();

    // Create test directory
    fs.mkdirSync(TEST_CONFIG_DIR, { recursive: true });

    // Mock homedir to use test directory
    (os as { homedir: () => string }).homedir = () => TEST_CONFIG_DIR;
  });

  afterEach(() => {
    // Restore original homedir
    (os as { homedir: () => string }).homedir = originalHomedir;

    // Clean up test directory
    try {
      fs.rmSync(TEST_CONFIG_DIR, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }

    _resetStore();
  });

  describe('saveConfig + loadConfig roundtrip', () => {
    it('saves and reads back config correctly', () => {
      const testPat = 'ghp_testPatToken123456789';
      const testKey = 'AIzaSyTestGeminiKey123456';
      const testDbPath = '/custom/path/to/db.sqlite';

      const saveResult = saveConfig({
        githubPat: testPat,
        geminiKey: testKey,
        dbPath: testDbPath,
      });

      expect(saveResult.success).toBe(true);

      const loaded = loadConfig();

      expect(loaded.githubPat).toBe(testPat);
      expect(loaded.geminiKey).toBe(testKey);
      expect(loaded.dbPath).toBe(testDbPath);
    });

    it('handles partial config updates', () => {
      saveConfig({ githubPat: 'initial_pat' });
      saveConfig({ geminiKey: 'initial_key' });

      const loaded = loadConfig();
      expect(loaded.githubPat).toBe('initial_pat');
      expect(loaded.geminiKey).toBe('initial_key');
    });
  });

  describe('encryption', () => {
    it('stores githubPat in encrypted form (not plaintext)', () => {
      const plaintextPat = 'ghp_mySecretToken123456789';

      saveConfig({ githubPat: plaintextPat });

      const rawValue = _getRawValue('githubPat');

      // Raw value should exist
      expect(rawValue).toBeDefined();
      // Raw value should NOT be the plaintext
      expect(rawValue).not.toBe(plaintextPat);
      // Raw value should be a non-empty string (encrypted)
      expect(typeof rawValue).toBe('string');
      expect(rawValue!.length).toBeGreaterThan(0);

      // But loading should return the decrypted value
      const loaded = loadConfig();
      expect(loaded.githubPat).toBe(plaintextPat);
    });

    it('stores geminiKey in encrypted form (not plaintext)', () => {
      const plaintextKey = 'AIzaSyMySecretGeminiApiKey';

      saveConfig({ geminiKey: plaintextKey });

      const rawValue = _getRawValue('geminiKey');

      expect(rawValue).toBeDefined();
      expect(rawValue).not.toBe(plaintextKey);
      expect(typeof rawValue).toBe('string');
      expect(rawValue!.length).toBeGreaterThan(0);

      const loaded = loadConfig();
      expect(loaded.geminiKey).toBe(plaintextKey);
    });
  });

  describe('isConfigured', () => {
    it('returns false when config is empty', () => {
      expect(isConfigured()).toBe(false);
    });

    it('returns false when only githubPat is set', () => {
      saveConfig({ githubPat: 'ghp_token' });
      expect(isConfigured()).toBe(false);
    });

    it('returns false when only geminiKey is set', () => {
      saveConfig({ geminiKey: 'AIzaSy_key' });
      expect(isConfigured()).toBe(false);
    });

    it('returns true after saving both keys', () => {
      saveConfig({
        githubPat: 'ghp_token123',
        geminiKey: 'AIzaSy_key456',
      });
      expect(isConfigured()).toBe(true);
    });
  });

  describe('clearConfig', () => {
    it('removes all stored values', () => {
      saveConfig({
        githubPat: 'ghp_token',
        geminiKey: 'AIzaSy_key',
        dbPath: '/custom/path',
      });

      expect(isConfigured()).toBe(true);

      const clearResult = clearConfig();
      expect(clearResult.success).toBe(true);

      const loaded = loadConfig();
      expect(loaded.githubPat).toBeNull();
      expect(loaded.geminiKey).toBeNull();
      // dbPath should return default after clear
      expect(loaded.dbPath).toContain('.repog');
    });
  });
});
