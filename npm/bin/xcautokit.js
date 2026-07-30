#!/usr/bin/env node
'use strict';

const { spawn } = require('child_process');
const fs = require('fs');
const { resolveBinary } = require('../scripts/resolve-binary');

async function main() {
  const args = process.argv.slice(2);
  let bin;
  try {
    bin = resolveBinary();
  } catch (err) {
    console.error(err.message || err);
    process.exit(1);
  }

  if (!fs.existsSync(bin)) {
    console.error(`xcautokit binary missing at ${bin}. Run: npm run build:go`);
    process.exit(1);
  }

  const child = spawn(bin, args.length ? args : ['mcp'], {
    stdio: 'inherit',
    env: process.env,
  });

  child.on('exit', (code, signal) => {
    if (signal) process.kill(process.pid, signal);
    process.exit(code == null ? 1 : code);
  });
}

main();
