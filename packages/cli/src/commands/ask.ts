import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';

/**
 * Register the `repog ask` command.
 * Ask questions about your repositories using RAG.
 */
export function register(program: Command): void {
  program
    .command('ask <question>')
    .description('Ask questions about your repositories using AI')
    .option('-r, --repo <name>', 'Ask about specific repository (owner/repo)')
    .option('--no-context', 'Ask without repository context')
    .option('--json', 'Output results as JSON')
    .action(async (_question, _options) => {
      const spinner = ora();
      spinner.info(chalk.yellow('repog ask — not yet implemented'));
    });
}
