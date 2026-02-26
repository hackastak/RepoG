import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog sync` command.
 * Syncs repositories from GitHub to the local database.
 */
export function register(program: Command): void {
  program
    .command('sync')
    .description('Sync repositories from GitHub to local database')
    .option('--full', 'Full sync (ignore last cursor)')
    .option('--owned', 'Only sync owned repositories')
    .option('--starred', 'Only sync starred repositories')
    .option('-l, --limit <number>', 'Limit number of repos to sync', parseInt)
    .option('--resume', 'Resume interrupted sync')
    .action(async (_options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog sync — not yet implemented'));
    });
}
