import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog status` command.
 * Shows current sync and embedding status.
 */
export function register(program: Command): void {
  program
    .command('status')
    .description('Show sync and embedding status')
    .option('--json', 'Output results as JSON')
    .action(async (_options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog status — not yet implemented'));
    });
}
