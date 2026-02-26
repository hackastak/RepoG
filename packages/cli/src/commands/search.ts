import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog search` command.
 * Performs semantic search across repositories.
 */
export function register(program: Command): void {
  program
    .command('search <query>')
    .description('Semantic search across your repositories')
    .option('-l, --limit <number>', 'Maximum results to return', parseInt, 10)
    .option('--lang <language>', 'Filter by primary language')
    .option('--starred', 'Only search starred repositories')
    .option('--owned', 'Only search owned repositories')
    .option('-r, --repo <name>', 'Search within specific repository (owner/repo)')
    .option('--json', 'Output results as JSON')
    .action(async (_query, _options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog search — not yet implemented'));
    });
}
