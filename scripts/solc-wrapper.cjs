#!/usr/bin/env node
// Native solc CLI adapter backed by the EXACT npm-pinned solc compiler.
// This keeps local/CI tests independent of Foundry's solc download service.
const solc = require('solc');
const fs = require('node:fs');
if (process.argv.includes('--version')) {
  process.stdout.write(`solc, the solidity compiler commandline interface\nVersion: ${solc.version()}\n`);
} else if (process.argv.includes('--standard-json')) {
  const input = fs.readFileSync(0, 'utf8');
  process.stdout.write(solc.compile(input));
} else {
  process.stderr.write('Only --version and --standard-json are supported\n');
  process.exitCode = 1;
}
