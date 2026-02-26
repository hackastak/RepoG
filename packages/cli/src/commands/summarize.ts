import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog summarize` command.
 * Generate an AI summary of a repository.
 */
export function register(program: Command): void {
  program
    .command('summarize <repo>')
    .description('Generate an AI summary of a repository')
    .option('--refresh', 'Regenerate summary even if cached')
    .option('--json', 'Output results as JSON')
    .action(async (_repo, _options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog summarize — not yet implemented'));
    });
}
