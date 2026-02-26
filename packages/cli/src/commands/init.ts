import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog init` command.
 * Initializes RepoG with GitHub and Gemini credentials.
 */
export function register(program: Command): void {
  program
    .command('init')
    .description('Initialize RepoG with your GitHub and Gemini API credentials')
    .option('--github-token <token>', 'GitHub Personal Access Token')
    .option('--gemini-key <key>', 'Google Gemini API Key')
    .option('--db-path <path>', 'Custom database path')
    .option('-f, --force', 'Overwrite existing configuration')
    .action(async (_options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog init — not yet implemented'));
    });
}
