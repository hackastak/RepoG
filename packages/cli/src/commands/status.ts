import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import os from 'os';
import { getStatus, isConfigured, formatRelativeTime } from '@repog/core';

/**
 * Register the `repog status` command.
 * Shows current sync and embedding status.
 */
export function register(program: Command): void {
  program
    .command('status')
    .description('Show sync and embedding status')
    .option('--json', 'Output results as JSON')
    .action(async (options) => {
      // check configuration first
      if (!isConfigured()) {
        console.error(chalk.red('Run `repog init` first.'));
        process.exit(1);
      }

      const spinner = ora('Fetching status...').start();

      // Handle Ctrl+C
      const handleSigInt = () => {
        spinner.stop();
        process.exit(0);
      };
      process.on('SIGINT', handleSigInt);

      try {
        const result = await getStatus();
        process.off('SIGINT', handleSigInt);

        if (options.json) {
          spinner.stop();
          console.log(JSON.stringify(result, null, 2));
          return;
        }

        spinner.stop();

        // Format plain text output
        const LABEL_WIDTH = 18;
        const padLabel = (label: string) => label.padEnd(LABEL_WIDTH);
        
        // Let's define a helper for consistent row formatting
        // Label:            Value
        // Total:              142
        // Owned:               23
        
        const formatRow = (label: string, value: string | number) => {
           return `    ${padLabel(label)}${String(value).padStart(15)}`; 
        };

        console.log(chalk.bold('RepoG Status'));
        console.log(chalk.gray('─────────────────────────────────────────────'));
        console.log('');

        // Repositories
        console.log(chalk.bold('  Repositories'));
        console.log(formatRow('Total:', result.repos.total));
        console.log(formatRow('Owned:', result.repos.owned));
        console.log(formatRow('Starred:', result.repos.starred));
        console.log(formatRow('Embedded:', result.repos.embeddedCount));
        console.log(formatRow('Pending embed:', result.repos.pendingEmbed));
        console.log('');

        // Knowledge Base
        console.log(chalk.bold('  Knowledge Base'));
        console.log(formatRow('Chunks:', result.embed.totalChunks));
        console.log(formatRow('Embeddings:', result.embed.totalEmbeddings));
        console.log('');

        // Last Sync
        console.log(chalk.bold('  Last Sync'));
        const syncStatus = result.sync.lastSyncStatus || 'Never synced';
        // Colorize status
        let statusColor = chalk.white;
        if (syncStatus === 'completed') statusColor = chalk.green;
        else if (syncStatus === 'failed') statusColor = chalk.red;
        else if (syncStatus === 'in_progress') statusColor = chalk.yellow;
        
        console.log(formatRow('Status:', statusColor(syncStatus)));
        
        if (result.sync.lastSyncedAt) {
          console.log(formatRow('Date:', formatRelativeTime(result.sync.lastSyncedAt)));
        } else if (syncStatus === 'Never synced') {
           // If never synced, date is redundant or implicitly never
           // Instructions say: "If lastSyncedAt is null, show `  Never synced`"
           // Wait, "show `  Never synced`" -> implies replacing the whole section content?
           // "If lastSyncedAt is null, show `  Never synced`"
           // Let's assume it means under "Last Sync" header.
           // My implementation shows "Status: Never synced" if status is null.
           // If status is null, lastSyncedAt is likely null.
           // If status is present but lastSyncedAt is null (unlikely), show what?
           // Let's stick to showing Date if present.
        }
        console.log('');

        // Last Embed
        console.log(chalk.bold('  Last Embed'));
        if (result.embed.lastEmbeddedAt) {
          console.log(formatRow('Date:', formatRelativeTime(result.embed.lastEmbeddedAt)));
        } else {
          console.log(`    ${padLabel('Date:')}${'Never embedded'.padStart(15)}`);
        }
        console.log('');

        // GitHub API
        console.log(chalk.bold('  GitHub API'));
        if (result.rateLimit) {
          const { remaining, limit, resetAt } = result.rateLimit;
          const remainingStr = `${remaining.toLocaleString()} / ${limit.toLocaleString()}`;
          console.log(formatRow('Remaining:', remainingStr));
          console.log(formatRow('Resets:', formatRelativeTime(resetAt)));
        } else {
          console.log(formatRow('Status:', chalk.red('unavailable')));
        }
        console.log('');

        // Database
        console.log(chalk.bold('  Database'));
        // Replace home dir with ~
        const homeDir = os.homedir();
        let displayPath = result.db.path;
        if (displayPath.startsWith(homeDir)) {
          displayPath = displayPath.replace(homeDir, '~');
        }
        console.log(formatRow('Path:', displayPath));
        console.log(formatRow('Size:', result.db.sizeMb));
        console.log('');

        console.log(chalk.gray('─────────────────────────────────────────────'));
        
        // Format generated at time (just time part)
        const timeStr = new Date(result.generatedAt).toLocaleTimeString('en-US', { hour12: false });
        console.log(chalk.gray(`Generated at ${timeStr}`));

      } catch (error) {
        spinner.stop();
        // If not configured error wasn't caught earlier (unlikely due to check), handle it
        if (error instanceof Error && error.name === 'NotConfiguredError') {
          console.error(chalk.red('Run `repog init` first.'));
          process.exit(1);
        }
        console.error(chalk.red('Error fetching status:'), error instanceof Error ? error.message : String(error));
        process.exit(1);
      }
    });
}
