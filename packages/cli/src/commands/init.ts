import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { input, password, confirm } from '@inquirer/prompts';
import {
  saveConfig,
  loadConfig,
  isConfigured,
  initDb,
  validateGitHubToken,
  validateGeminiKey,
  getDefaultDbPath,
} from '@repog/core';

/**
 * Options passed from command line.
 */
interface InitOptions {
  githubToken?: string;
  geminiKey?: string;
  dbPath?: string;
  force?: boolean;
}

/**
 * Handle Ctrl+C (SIGINT) gracefully.
 * Prints a message and exits with code 0.
 */
function setupSignalHandlers(): void {
  const handler = (): void => {
    console.log('\nSetup cancelled.');
    process.exit(0);
  };

  process.on('SIGINT', handler);
  process.on('SIGTERM', handler);
}

/**
 * Mask a token/key for safe display.
 * Shows first 4 and last 4 characters with asterisks in between.
 */
function maskSecret(secret: string): string {
  if (secret.length <= 8) {
    return '****';
  }
  return `${secret.slice(0, 4)}${'*'.repeat(8)}${secret.slice(-4)}`;
}

/**
 * Run the init command logic.
 */
async function runInit(options: InitOptions): Promise<void> {
  setupSignalHandlers();

  const spinner = ora();

  console.log(chalk.bold('\nRepoG Setup\n'));

  // Check if already configured
  if (isConfigured() && !options.force) {
    const config = loadConfig();
    console.log(chalk.yellow('RepoG is already configured.'));
    console.log(`  GitHub: ${config.githubPat ? chalk.green('configured') : chalk.red('not set')}`);
    console.log(`  Gemini: ${config.geminiKey ? chalk.green('configured') : chalk.red('not set')}`);
    console.log(`  Database: ${config.dbPath}`);
    console.log('');

    const overwrite = await confirm({
      message: 'Do you want to reconfigure?',
      default: false,
    });

    if (!overwrite) {
      console.log(chalk.gray('Setup cancelled.'));
      return;
    }
  }

  // Get GitHub token
  let githubToken = options.githubToken;
  if (!githubToken) {
    console.log(chalk.dim('Create a GitHub PAT at: https://github.com/settings/tokens'));
    console.log(chalk.dim('Required scope: repo\n'));

    githubToken = await password({
      message: 'GitHub Personal Access Token:',
      mask: '*',
      validate: (value) => {
        if (!value || value.trim() === '') {
          return 'Token is required';
        }
        return true;
      },
    });
  }

  // Validate GitHub token
  spinner.start('Validating GitHub token...');
  const githubResult = await validateGitHubToken(githubToken);

  if (!githubResult.valid) {
    spinner.fail(chalk.red(`GitHub token invalid: ${githubResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green(`GitHub token valid (${githubResult.login})`));

  // Get Gemini API key
  let geminiKey = options.geminiKey;
  if (!geminiKey) {
    console.log('');
    console.log(chalk.dim('Get a Gemini API key at: https://aistudio.google.com/apikey\n'));

    geminiKey = await password({
      message: 'Gemini API Key:',
      mask: '*',
      validate: (value) => {
        if (!value || value.trim() === '') {
          return 'API key is required';
        }
        return true;
      },
    });
  }

  // Validate Gemini API key
  spinner.start('Validating Gemini API key...');
  const geminiResult = await validateGeminiKey(geminiKey);

  if (!geminiResult.valid) {
    spinner.fail(chalk.red(`Gemini API key invalid: ${geminiResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green('Gemini API key valid'));

  // Get database path
  const defaultDbPath = getDefaultDbPath();
  let dbPath = options.dbPath;

  if (!dbPath) {
    console.log('');
    dbPath = await input({
      message: 'Database path:',
      default: defaultDbPath,
    });
  }

  // Initialize database
  spinner.start('Initializing database...');
  const dbResult = initDb(dbPath);

  if (!dbResult.success) {
    spinner.fail(chalk.red(`Database init failed: ${dbResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green(`Database initialized (${dbResult.tablesCreated.length} tables)`));

  // Save configuration
  spinner.start('Saving configuration...');
  const saveResult = saveConfig({
    githubPat: githubToken,
    geminiKey: geminiKey,
    dbPath: dbPath,
  });

  if (!saveResult.success) {
    spinner.fail(chalk.red(`Failed to save config: ${saveResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green('Configuration saved'));

  // Summary
  console.log('');
  console.log(chalk.bold.green('Setup complete!'));
  console.log('');
  console.log('Configuration:');
  console.log(`  GitHub: ${chalk.cyan(maskSecret(githubToken))} (${githubResult.login})`);
  console.log(`  Gemini: ${chalk.cyan(maskSecret(geminiKey))}`);
  console.log(`  Database: ${chalk.cyan(dbPath)}`);
  console.log('');
  console.log('Next steps:');
  console.log(`  ${chalk.cyan('repog sync')}     - Sync your GitHub repositories`);
  console.log(`  ${chalk.cyan('repog status')}   - Check sync status`);
  console.log('');
}

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
    .action(async (options: InitOptions) => {
      try {
        await runInit(options);
      } catch (error) {
        // Handle ExitPromptError from inquirer (Ctrl+C)
        if (error instanceof Error && error.name === 'ExitPromptError') {
          console.log('\nSetup cancelled.');
          process.exit(0);
        }
        throw error;
      }
    });
}
