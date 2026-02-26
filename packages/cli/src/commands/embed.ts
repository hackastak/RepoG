import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog embed` command.
 * Generates embeddings for repository content.
 */
export function register(program: Command): void {
  program
    .command('embed')
    .description('Generate embeddings for repository content')
    .option('-f, --force', 'Re-embed all repositories')
    .option('-r, --repo <name>', 'Embed specific repository (owner/repo)')
    .option('--dry-run', 'Show what would be embedded without running')
    .action(async (_options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog embed — not yet implemented'));
    });
}
