import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog recommend` command.
 * Gets AI-powered repository recommendations.
 */
export function register(program: Command): void {
  program
    .command('recommend <query>')
    .description('Get AI-powered repository recommendations')
    .option('-l, --limit <number>', 'Maximum recommendations to return', parseInt, 5)
    .option('--starred', 'Only recommend from starred repositories')
    .option('--owned', 'Only recommend from owned repositories')
    .option('--json', 'Output results as JSON')
    .action(async (_query, _options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog recommend — not yet implemented'));
    });
}
