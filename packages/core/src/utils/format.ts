/**
 * Text formatting utilities for RepoG.
 */

/**
 * Options for word wrapping text.
 */
export interface WrapTextOptions {
  /** Maximum width for each line (default: 80) */
  width?: number;
  /** Indent string for continuation lines (default: '') */
  indent?: string;
  /** Indent string for the first line (default: '') */
  firstLineIndent?: string;
}

/**
 * Wrap text to a specified width with optional indentation.
 *
 * This function wraps text at word boundaries to fit within a maximum width.
 * Continuation lines can have a different indent than the first line.
 *
 * @param text - The text to wrap
 * @param options - Wrapping options
 * @returns The wrapped text with line breaks
 */
export function wrapText(text: string, options: WrapTextOptions = {}): string {
  const { width = 80, indent = '', firstLineIndent = '' } = options;

  // Normalize whitespace and split into words
  const words = text.replace(/\s+/g, ' ').trim().split(' ');

  if (words.length === 0 || (words.length === 1 && words[0] === '')) {
    return '';
  }

  const lines: string[] = [];
  let currentLine = firstLineIndent;
  let isFirstLine = true;

  for (const word of words) {
    const lineIndent = isFirstLine ? firstLineIndent : indent;
    const testLine = currentLine.length === lineIndent.length
      ? currentLine + word
      : currentLine + ' ' + word;

    if (testLine.length <= width) {
      currentLine = testLine;
    } else {
      // Line would exceed width
      if (currentLine.length > lineIndent.length) {
        // Current line has content, push it and start new line
        lines.push(currentLine);
        isFirstLine = false;
        currentLine = indent + word;
      } else {
        // Word is longer than available width, add it anyway
        currentLine = lineIndent + word;
      }
    }
  }

  // Push the last line if it has content
  if (currentLine.length > 0) {
    lines.push(currentLine);
  }

  return lines.join('\n');
}

/**
 * Format a star count for display with comma separators.
 *
 * @param stars - The number of stars
 * @returns Formatted star count string
 */
export function formatStars(stars: number): string {
  return stars.toLocaleString();
}

/**
 * Format a similarity score as a percentage.
 *
 * @param similarity - Similarity score between 0 and 1
 * @returns Formatted percentage string (e.g., "85.3%")
 */
export function formatSimilarity(similarity: number): string {
  return `${(similarity * 100).toFixed(1)}%`;
}

/**
 * Truncate text to a maximum length with ellipsis.
 *
 * @param text - The text to truncate
 * @param maxLength - Maximum length including ellipsis
 * @returns Truncated text
 */
export function truncateText(text: string, maxLength: number = 200): string {
  const cleaned = text.replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
  if (cleaned.length <= maxLength) {
    return cleaned;
  }
  return cleaned.slice(0, maxLength - 3) + '...';
}

/**
 * Format a relative time string (e.g., "2 hours ago", "in 14 minutes").
 *
 * @param isoString - The ISO datetime string to format
 * @returns formatted relative time string
 */
export function formatRelativeTime(isoString: string): string {
  const date = new Date(isoString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = diffMs / 1000;

  if (diffMs >= 0) {
    // Past
    if (diffSec < 60) {
      return 'just now';
    }
    const minutes = Math.floor(diffSec / 60);
    if (minutes < 60) {
      return `${minutes} minute${minutes !== 1 ? 's' : ''} ago`;
    }
    const hours = Math.floor(minutes / 60);
    if (hours < 24) {
      return `${hours} hour${hours !== 1 ? 's' : ''} ago`;
    }
    const days = Math.floor(hours / 24);
    return `${days} day${days !== 1 ? 's' : ''} ago`;
  } else {
    // Future
    const absDiffSec = Math.abs(diffSec);
    const minutes = Math.ceil(absDiffSec / 60); // Round up for "in X minutes" so 59.1s becomes 1 min

    if (minutes < 60) {
      return `in ${minutes} minute${minutes !== 1 ? 's' : ''}`;
    }
    const hours = Math.floor(minutes / 60);
    if (hours < 24) {
      return `in ${hours} hour${hours !== 1 ? 's' : ''}`;
    }
    const days = Math.floor(hours / 24);
    return `in ${days} day${days !== 1 ? 's' : ''}`;
  }
}

