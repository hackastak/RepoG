import { Command } from 'commander';
import { createRequire } from 'module';

import { register as registerInit } from './commands/init.js';
import { register as registerSync } from './commands/sync.js';
import { register as registerEmbed } from './commands/embed.js';
import { register as registerSearch } from './commands/search.js';
import { register as registerRecommend } from './commands/recommend.js';
import { register as registerAsk } from './commands/ask.js';
import { register as registerSummarize } from './commands/summarize.js';
import { register as registerStatus } from './commands/status.js';

const require = createRequire(import.meta.url);
const packageJson = require('../package.json') as { version: string };

const program = new Command();

program
  .name('repog')
  .description('AI-powered knowledge base for your GitHub repositories')
  .version(packageJson.version);

// Register all commands
registerInit(program);
registerSync(program);
registerEmbed(program);
registerSearch(program);
registerRecommend(program);
registerAsk(program);
registerSummarize(program);
registerStatus(program);

// Parse command line arguments
program.parseAsync(process.argv);
