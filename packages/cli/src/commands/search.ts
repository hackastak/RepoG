import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import {
  isConfigured,
  loadConfig,
  getDb,
  searchRepos,
  type SearchFilters,
  type VectorSearchResult,
  type SearchQueryResult,
} from '@repog/core';

/**
 * Options passed from command line.
 */
interface SearchCommandOptions {
  limit?: number;
  language?: string;
  starred?: boolean;
  owned?: boolean;
  owner?: string;
}

/**
 * Check if the chunk_embeddings table has at least one row.
 */
function hasEmbeddings(): boolean {
  try {
    const config = loadConfig();
    const db = getDb(config.dbPath);
    const row = db.prepare('SELECT COUNT(*) as count FROM chunk_embeddings').get() as {
      count: number;
    };
    return row.count > 0;
  } catch {
    return false;
  }
}

/**
 * Format similarity score as percentage.
 */
function formatSimilarity(similarity: number): string {
  return `${(similarity * 100).toFixed(1)}%`;
}

/**
 * Format stars count with comma separators.
 */
function formatStars(stars: number): string {
  return stars.toLocaleString();
}

/**
 * Truncate content to a reasonable length for display.
 */
function truncateContent(content: string, maxLength: number = 200): string {
  const cleaned = content.replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
  if (cleaned.length <= maxLength) {
    return cleaned;
  }
  return cleaned.slice(0, maxLength - 3) + '...';
}

/**
 * Render a single search result.
 */
function renderResult(result: VectorSearchResult, index: number): void {
  const header = `${index + 1}. ${chalk.bold.cyan(result.repoFullName)}`;
  const similarity = chalk.green(formatSimilarity(result.similarity));
  const stars = chalk.yellow(`★ ${formatStars(result.stars)}`);

  // Build language badge if present
  const langBadge = result.language ? chalk.magenta(`[${result.language}]`) : '';

  // Build ownership badges
  const badges: string[] = [];
  if (result.isOwned) badges.push(chalk.blue('owned'));
  if (result.isStarred) badges.push(chalk.yellow('starred'));
  const badgeStr = badges.length > 0 ? chalk.gray(`(${badges.join(', ')})`) : '';

  console.log(`\n${header} ${similarity} ${stars} ${langBadge} ${badgeStr}`);

  if (result.description) {
    console.log(chalk.gray(`   ${result.description}`));
  }

  console.log(chalk.gray(`   ${result.htmlUrl}`));

  // Show matched chunk content
  const chunkLabel = chalk.dim(`[${result.chunkType}]`);
  const content = truncateContent(result.content);
  console.log(`   ${chunkLabel} ${chalk.white(content)}`);
}

/**
 * Render the search results.
 */
function renderResults(queryResult: SearchQueryResult, query: string): void {
  const { results, totalConsidered, queryEmbeddingMs, searchMs } = queryResult;

  if (results.length === 0) {
    console.log(chalk.yellow('\nNo matching repositories found.'));
    return;
  }

  console.log(
    chalk.bold(`\nFound ${results.length} matching ${results.length === 1 ? 'repository' : 'repositories'}:`)
  );

  for (let i = 0; i < results.length; i++) {
    renderResult(results[i], i);
  }

  // Show timing metrics
  console.log('');
  console.log(
    chalk.dim(
      `Query: "${query}" | Chunks searched: ${totalConsidered} | Embedding: ${Math.round(queryEmbeddingMs)}ms | Search: ${Math.round(searchMs)}ms`
    )
  );
}

/**
 * Run the search command.
 */
async function runSearch(query: string, options: SearchCommandOptions): Promise<void> {
  // Check if configured
  if (!isConfigured()) {
    console.log(chalk.red('Run `repog init` first.'));
    process.exit(1);
  }

  // Check if embeddings exist
  if (!hasEmbeddings()) {
    console.log(chalk.red('No embeddings found. Run `repog sync` then `repog embed` first.'));
    process.exit(1);
  }

  // Build search filters
  const filters: SearchFilters = {
    limit: options.limit ?? 3,
  };

  if (options.language) {
    filters.language = options.language;
  }

  if (options.starred) {
    filters.starred = true;
  }

  if (options.owned) {
    filters.owned = true;
  }

  if (options.owner) {
    filters.owner = options.owner;
  }

  // Run search with spinner
  const spinner = ora('Searching...').start();

  try {
    const result = await searchRepos(query, filters);
    spinner.stop();

    renderResults(result, query);
  } catch (error) {
    spinner.fail(chalk.red('Search failed'));
    if (error instanceof Error) {
      console.error(chalk.red(`  Error: ${error.message}`));
    }
    process.exit(1);
  }
}

/**
 * Register the `repog search` command.
 * Performs semantic search across repositories.
 */
export function register(program: Command): void {
  program
    .command('search <query>')
    .description('Semantic search across your repositories')
    .option('-l, --limit <number>', 'Maximum results to return', parseInt, 3)
    .option('--language <lang>', 'Filter by primary language')
    .option('--starred', 'Only search starred repositories')
    .option('--owned', 'Only search owned repositories')
    .option('--owner <owner>', 'Filter by repo owner username')
    .action(async (query: string, options: SearchCommandOptions) => {
      await runSearch(query, options);
    });
}
