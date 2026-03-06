import { describe, it, expect } from 'vitest';
import { formatRelativeTime } from './format.js';

describe('formatRelativeTime', () => {
  it('returns "just now" for timestamps within the last 60 seconds', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 30 * 1000); // 30 seconds ago
    expect(formatRelativeTime(past.toISOString())).toBe('just now');
  });

  it('returns "X minutes ago" for timestamps 1–59 minutes ago', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 5 * 60 * 1000); // 5 minutes ago
    expect(formatRelativeTime(past.toISOString())).toBe('5 minutes ago');

    const past2 = new Date(now.getTime() - 59 * 60 * 1000); // 59 minutes ago
    expect(formatRelativeTime(past2.toISOString())).toBe('59 minutes ago');
  });

  it('returns "X hours ago" for timestamps 1–23 hours ago', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 2 * 60 * 60 * 1000); // 2 hours ago
    expect(formatRelativeTime(past.toISOString())).toBe('2 hours ago');

    const past2 = new Date(now.getTime() - 23 * 60 * 60 * 1000); // 23 hours ago
    expect(formatRelativeTime(past2.toISOString())).toBe('23 hours ago');
  });

  it('returns "X days ago" for timestamps 1+ days ago', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000); // 2 days ago
    expect(formatRelativeTime(past.toISOString())).toBe('2 days ago');
  });

  it('returns "in X minutes" for future timestamps', () => {
    const now = new Date();
    const future = new Date(now.getTime() + 15 * 60 * 1000); // in 15 minutes
    expect(formatRelativeTime(future.toISOString())).toBe('in 15 minutes');
  });

  it('returns "in X hours" for future timestamps more than 60 minutes away', () => {
    const now = new Date();
    const future = new Date(now.getTime() + 2 * 60 * 60 * 1000); // in 2 hours
    expect(formatRelativeTime(future.toISOString())).toBe('in 2 hours');
  });

  it('handles singular units correctly', () => {
    const now = new Date();
    
    // 1 minute ago
    const pastMin = new Date(now.getTime() - 60 * 1000);
    expect(formatRelativeTime(pastMin.toISOString())).toBe('1 minute ago');

    // 1 hour ago
    const pastHour = new Date(now.getTime() - 60 * 60 * 1000);
    expect(formatRelativeTime(pastHour.toISOString())).toBe('1 hour ago');

    // 1 day ago
    const pastDay = new Date(now.getTime() - 24 * 60 * 60 * 1000);
    expect(formatRelativeTime(pastDay.toISOString())).toBe('1 day ago');

    // in 1 minute
    const futureMin = new Date(now.getTime() + 60 * 1000);
    expect(formatRelativeTime(futureMin.toISOString())).toBe('in 1 minute');

    // in 1 hour
    const futureHour = new Date(now.getTime() + 60 * 60 * 1000);
    expect(formatRelativeTime(futureHour.toISOString())).toBe('in 1 hour');
  });
});
